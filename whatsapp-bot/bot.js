const {
  makeWASocket, useMultiFileAuthState, DisconnectReason,
  downloadMediaMessage, getContentType,
  makeCacheableSignalKeyStore, fetchLatestBaileysVersion,
} = require('baileys');
const pino = require('pino');
const qrcode = require('qrcode-terminal');
const fs = require('fs');
const path = require('path');

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080';
const AUTH_DIR = process.env.AUTH_DIR || './auth_state';
const logger = pino({ level: 'silent' });

// ─── State ───
let enabledGroups = new Set();   // WA group JIDs that Reyna is active in
let groupModes = new Map();      // groupJid → "auto" | "reaction"
const groupFiles = new Map();    // groupJid → Map<msgId, fileInfo>
const groupLastSyncTime = new Map(); // groupJid → timestamp (throttle re-syncs)

// dmContext caches per-user conversation state for private DM follow-ups.
// Keyed by senderPhone. Stores the most recent search results so messages
// like "the second one", "send me that wien bridge file", "what about the
// PYQ?" can resolve back to a concrete file without re-querying the LLM.
const dmContext = new Map(); // senderPhone → { lastQuery, lastFiles, lastReply, ts }

function setDmContext(phone, ctx) {
  dmContext.set(phone, { ...ctx, ts: Date.now() });
  // Auto-expire after 30 minutes
  setTimeout(() => {
    const c = dmContext.get(phone);
    if (c && c.ts === ctx.ts) dmContext.delete(phone);
  }, 30 * 60 * 1000);
}
function getDmContext(phone) {
  return dmContext.get(phone) || null;
}

// hasWakeWord returns true if the message begins with a Reyna wake word.
// In private DMs Reyna only responds when explicitly addressed (or a recent
// active session is in progress — see ACTIVE_SESSION_TTL).
function hasWakeWord(text) {
  const t = text.toLowerCase().trim();
  return t.startsWith('reyna') || t.startsWith('@reyna') || t.startsWith('hey reyna');
}

// activeSessions tracks senders who recently received a reply from Reyna.
// While a session is active, follow-up messages from that sender are
// processed WITHOUT requiring the "reyna" wake word — so things like "1",
// "the second one", "in simpler words", "tell me more" Just Work.
const ACTIVE_SESSION_TTL = 10 * 60 * 1000; // 10 minutes
const activeSessions = new Map(); // senderJid → lastReplyTs
function markSessionActive(jid) { activeSessions.set(jid, Date.now()); }
function isSessionActive(jid) {
  const ts = activeSessions.get(jid);
  if (!ts) return false;
  if (Date.now() - ts > ACTIVE_SESSION_TTL) {
    activeSessions.delete(jid);
    return false;
  }
  return true;
}

// stripWakeWord removes the leading wake word + any trailing punctuation so
// the rest of the message can be processed as the actual query.
function stripWakeWord(text) {
  let t = text.trim();
  for (const prefix of ['hey reyna,', 'hey reyna', '@reyna,', '@reyna', 'reyna,', 'reyna:', 'reyna']) {
    if (t.toLowerCase().startsWith(prefix)) {
      t = t.slice(prefix.length).trim();
      t = t.replace(/^[,:\-\s]+/, '');
      return t;
    }
  }
  return t;
}

// BOT_MARK is an invisible zero-width-space prefix the bot stamps on all
// outgoing DM text replies. The bot is typically linked to the user's own
// WhatsApp account, so when our reply echoes back through messages.upsert
// it arrives with fromMe=true — same as the user typing in their own chat.
// We can't tell them apart by fromMe alone, so we tag our own outgoing
// messages with this invisible marker. Anything fromMe that starts with
// the marker is OUR reply and gets skipped; anything else fromMe is the
// linked user actually typing and gets processed normally.
const BOT_MARK = '\u200B';
function tagOutgoing(payload) {
  if (payload && typeof payload.text === 'string' && !payload.text.startsWith(BOT_MARK)) {
    return { ...payload, text: BOT_MARK + payload.text };
  }
  return payload;
}
function isBotEcho(text) {
  return typeof text === 'string' && text.startsWith(BOT_MARK);
}

// dmSend wraps sock.sendMessage and tags the outgoing text with BOT_MARK so
// the inbound echo of our own reply can be recognised and ignored.
async function dmSend(sock, chat, payload) {
  return sock.sendMessage(chat, tagOutgoing(payload));
}

// botOwnerPhone is the linked WhatsApp account's phone number — i.e. the
// dashboard owner. We use this as the `user_phone` field in backend calls
// from DMs so retrieval scopes to the OWNER'S groups, not the random DM
// sender's groups (a friend who DMs the bot has no group membership).
let botOwnerPhone = '';
function setBotOwnerPhone(p) {
  botOwnerPhone = p;
  console.log(`  Bot owner phone: ${p}`);
}

function driveViewUrl(driveFileId) {
  if (!driveFileId || driveFileId.startsWith('local_') || driveFileId.startsWith('meta_')) return null;
  return `https://drive.google.com/file/d/${driveFileId}/view`;
}

// formatFilesForReply turns an array of file objects into a numbered markdown
// list with Drive links. Used in DM responses so the user can click straight
// through to the file.
function formatFilesForReply(files, max = 5) {
  if (!files || files.length === 0) return '';
  const lines = [];
  const slice = files.slice(0, max);
  slice.forEach((f, i) => {
    const url = driveViewUrl(f.drive_file_id);
    const num = i + 1;
    const sender = f.shared_by_name ? ` (by ${f.shared_by_name})` : '';
    if (url) {
      lines.push(`${num}. *${f.file_name}*${sender}\n   ${url}`);
    } else {
      lines.push(`${num}. *${f.file_name}*${sender}`);
    }
  });
  if (files.length > max) lines.push(`...and ${files.length - max} more`);
  return lines.join('\n');
}

// detectFollowupIndex parses messages like "send me the second one", "1",
// "the third", "first" → returns a 0-based index, or -1 if not a follow-up.
function detectFollowupIndex(text) {
  const t = text.toLowerCase().trim().replace(/[?.,!]/g, '');
  const wordMap = { first: 0, '1st': 0, one: 0, second: 1, '2nd': 1, two: 1,
                    third: 2, '3rd': 2, three: 2, fourth: 3, '4th': 3, four: 3,
                    fifth: 4, '5th': 4, five: 4 };
  // Just a number
  const num = parseInt(t, 10);
  if (!isNaN(num) && num >= 1 && num <= 9) return num - 1;
  for (const [w, idx] of Object.entries(wordMap)) {
    if (t === w || t.includes(`the ${w}`) || t.includes(`${w} one`) || t.startsWith(w + ' ')) return idx;
  }
  return -1;
}

// ─── Backend API ───

async function sendCommand(groupJid, command, userPhone, userName, extra) {
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/command`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        group_wa_id: groupJid, command,
        user_phone: userPhone, user_name: userName || '',
        file_name: extra?.fileName || '', file_size: extra?.fileSize || 0,
        mime_type: extra?.mimeType || '', subject: extra?.subject || '',
      }),
    });
    return await res.json();
  } catch (err) {
    console.error('Backend error:', err.message);
    return { reply: 'Could not reach backend.' };
  }
}

async function uploadFile(groupJid, userPhone, userName, fileInfo, fileBuffer) {
  try {
    const form = new FormData();
    form.append('group_wa_id', groupJid);
    form.append('user_phone', userPhone);
    form.append('user_name', userName || '');
    form.append('file_name', fileInfo.fileName);
    form.append('file_size', String(fileInfo.fileSize));
    form.append('mime_type', fileInfo.mimeType);
    form.append('subject', fileInfo.subject || '');
    form.append('file', new Blob([fileBuffer], { type: fileInfo.mimeType }), fileInfo.fileName);

    console.log(`  Upload: ${fileInfo.fileName} (${(fileBuffer.length / 1024).toFixed(0)}KB)`);

    const res = await fetch(`${BACKEND_URL}/api/bot/upload`, { method: 'POST', body: form });
    const data = await res.json();
    if (!res.ok) {
      console.error(`Upload HTTP ${res.status}:`, data);
      return { reply: `Upload failed: ${data.error || 'unknown'}` };
    }
    return data;
  } catch (err) {
    console.error('Upload error:', err.message);
    return { reply: 'File upload failed.' };
  }
}

async function nlpRetrieve(groupJid, userPhone, query) {
  try {
    const res = await fetch(`${BACKEND_URL}/api/nlp/retrieve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query, group_wa_id: groupJid, user_phone: userPhone }),
    });
    return await res.json();
  } catch (err) {
    console.error('NLP retrieve error:', err.message);
    return { reply: 'Could not process that query.', files: [] };
  }
}

async function notesQA(groupJid, userPhone, question, prevTurn) {
  try {
    const body = { question, group_wa_id: groupJid, user_phone: userPhone };
    if (prevTurn && prevTurn.question && prevTurn.answer) {
      body.previous_question = prevTurn.question;
      body.previous_answer = prevTurn.answer;
      if (prevTurn.sources) body.previous_sources = prevTurn.sources;
    }
    const res = await fetch(`${BACKEND_URL}/api/nlp/qa`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return await res.json();
  } catch (err) {
    console.error('Q&A error:', err.message);
    return { answer: 'Could not process that question.', sources: [] };
  }
}

async function sendReaction(groupJid, userPhone, userName, fileInfo) {
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/reaction`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        group_wa_id: groupJid,
        user_phone: userPhone,
        user_name: userName,
        file_name: fileInfo.fileName,
        file_size: fileInfo.fileSize || 0,
        mime_type: fileInfo.mimeType || '',
        emoji: fileInfo.emoji || '📌',
        wa_message_id: fileInfo.msgId || '',
      }),
    });
    return await res.json();
  } catch (err) {
    console.error('Reaction API error:', err.message);
    return { reply: 'Reaction processing failed.' };
  }
}

// ─── Group Allowlist ───

async function refreshEnabledGroups() {
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/enabled-groups`);
    const data = await res.json();
    enabledGroups = new Set(data.groups || []);
    console.log(`  Enabled groups: ${enabledGroups.size}`);
  } catch (err) {
    console.error('Failed to fetch enabled groups:', err.message);
  }
}

// Returns { mode, enabled } — the bot must check BOTH.
// mode='auto'|'reaction', enabled=true|false.
// On network failure, defaults to DISABLED (safe side — don't track if unsure).
async function getGroupMode(groupJid) {
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/group-mode?wa_id=${encodeURIComponent(groupJid)}`);
    const data = await res.json();
    return { mode: data.mode || 'auto', enabled: data.enabled === true };
  } catch (err) {
    console.log(`  [MODE] failed to fetch for ${groupJid}: ${err.message} — defaulting to disabled`);
    return { mode: 'auto', enabled: false };
  }
}

// ─── File Tracking ───

function getGroupTracker(groupJid) {
  if (!groupFiles.has(groupJid)) groupFiles.set(groupJid, new Map());
  return groupFiles.get(groupJid);
}

function trackFile(groupJid, msgId, info) {
  const tracker = getGroupTracker(groupJid);
  tracker.set(msgId, { ...info, staged: false, ts: Date.now() });
  // Auto-expire after 30 minutes
  setTimeout(() => {
    const t = getGroupTracker(groupJid);
    const f = t.get(msgId);
    if (f && f.ts === info.ts) t.delete(msgId);
  }, 30 * 60 * 1000);
}

function getUntrackedFiles(groupJid) {
  const tracker = getGroupTracker(groupJid);
  return [...tracker.entries()].filter(([, v]) => !v.staged).map(([k, v]) => ({ msgId: k, ...v }));
}

function getLastFile(groupJid) {
  const tracker = getGroupTracker(groupJid);
  let latest = null, latestKey = null;
  for (const [k, v] of tracker) {
    if (!latest || v.ts > latest.ts) { latest = v; latestKey = k; }
  }
  return latest ? { msgId: latestKey, ...latest } : null;
}

function markStaged(groupJid, msgId) {
  const tracker = getGroupTracker(groupJid);
  const f = tracker.get(msgId);
  if (f) f.staged = true;
}

function phoneFromJid(jid) {
  if (!jid) return '';
  const beforeAt = jid.split('@')[0];
  // Try all parts of the JID (some have format like "number:device@s.whatsapp.net")
  const parts = beforeAt.split(':');
  for (const part of parts) {
    const cleaned = part.replace(/\D/g, '');
    if (cleaned.length >= 10) {
      return '+' + cleaned;
    }
  }
  // This is a LID or device ID — not a real phone number
  // Return with a prefix so it's identifiable
  return parts[0];
}

function isRealPhoneNumber(phone) {
  if (!phone) return false;
  const cleaned = phone.replace(/\D/g, '');
  return cleaned.length >= 10;
}

// ─── Natural Language Intent Detection ───

function detectIntent(message) {
  const msg = message.toLowerCase().trim();

  // Strip wake word
  let stripped = msg;
  for (const prefix of ['reyna ', 'reyna, ', 'hey reyna ', '@reyna ']) {
    if (stripped.startsWith(prefix)) {
      stripped = stripped.slice(prefix.length).trim();
      break;
    }
  }

  // SAVE
  for (const p of ['save', 'add', 'stage', 'track', 'store', 'keep']) {
    if (stripped.includes(p)) return { intent: 'save', query: '' };
  }

  // PUSH / COMMIT
  for (const p of ['push', 'commit', 'upload', 'sync', 'send to drive']) {
    if (stripped.includes(p)) return { intent: 'push', query: '' };
  }

  // REMOVE
  for (const p of ['remove', 'delete', 'unstage', 'clear', 'rm']) {
    if (stripped.includes(p)) return { intent: 'remove', query: stripped };
  }

  // ── Q&A patterns FIRST — questions about content of notes ──
  // Must be checked BEFORE bare academic terms, otherwise "summarize module 5
  // notes" matches "module" and gets routed to keyword search.
  const qaPatterns = [
    'summarize', 'summarise', 'summary', 'summary of',
    'explain', 'samjhao', 'samjha', 'batao', 'samjhaiye',
    'what does', 'what is', 'what are', 'what was',
    'tell me about', 'tell me', 'describe', 'define', 'definition',
    'how does', 'how do', 'how is', 'why does', 'why is',
    'compare', 'difference between', 'derive', 'derivation', 'prove',
    'what did the teacher', 'what chapter', 'from our notes', 'from the notes',
    'according to', 'kya hai', 'kya bata', 'meaning of',
  ];
  for (const p of qaPatterns) {
    if (stripped.includes(p)) return { intent: 'qa', query: stripped };
  }

  // ── NLP retrieval patterns — questions about WHO shared WHAT WHEN ──
  // Must also be BEFORE academic-term fallback so "notes shared by rakesh"
  // routes to NLP retrieval, not legacy keyword search.
  const nlpPatterns = [
    'what did', 'who shared', 'who uploaded', 'has anyone shared',
    'do we have', 'is there', "what's new", 'whats new',
    'anything about', 'any files about', 'any notes',
    'files from', 'shared by', 'sent by', 'uploaded by',
    'kisne', 'kab bheja', 'kya bheja', 'aaj kya', 'kal kya',
  ];
  for (const p of nlpPatterns) {
    if (stripped.includes(p)) return { intent: 'nlp_retrieve', query: stripped };
  }

  // SEARCH / FIND
  for (const p of ['find', 'search', 'look for', 'where is', 'get me', 'show me', 'do you have', 'send me', 'drop me', 'need', 'dhundo', 'dikhao', 'bhejo']) {
    if (stripped.includes(p)) {
      const idx = stripped.indexOf(p);
      let query = stripped.slice(idx + p.length).trim().replace(/["'?.,!]/g, '');
      return { intent: 'search', query: query || stripped };
    }
  }

  // Bare academic terms → search (last-resort, only if nothing else matched)
  for (const t of ['notes', 'pyq', 'paper', 'assignment', 'module', 'unit', 'lab', 'slides', 'pdf', 'exam']) {
    if (stripped.includes(t)) return { intent: 'search', query: stripped };
  }

  // HISTORY / LOG
  for (const p of ['history', 'log', 'recent', 'latest', 'last', 'all files']) {
    if (stripped.includes(p)) return { intent: 'history', query: '' };
  }

  // STATUS
  for (const p of ['status', "what's new", 'whats new', 'update', 'how many', 'overview']) {
    if (stripped.includes(p)) return { intent: 'status', query: '' };
  }

  // HELP
  for (const p of ['help', 'how to', 'how do', 'what can you', 'commands', 'guide']) {
    if (stripped.includes(p)) return { intent: 'help', query: '' };
  }

  return { intent: 'unknown', query: '' };
}

function isReynaMessage(text) {
  const lower = text.toLowerCase().trim();
  // Starts with wake word
  if (lower.startsWith('reyna')) return true;
  if (lower.startsWith('@reyna')) return true;
  if (lower.startsWith('hey reyna')) return true;
  // Legacy slash commands still supported
  if (lower.startsWith('/reyna')) return true;
  return false;
}

// ─── On-Demand Group Sync ───
// Only registers a group with the backend when /reyna init is used.
const syncedGroups = new Set();

async function ensureGroupSynced(sock, groupJid) {
  let groupName = null;
  let memberCount = 0;

  // Single attempt — no retries. Retries cause extra groupMetadata calls
  // which trigger 440 disconnects.
  try {
    const metadata = await sock.groupMetadata(groupJid);
    if (metadata?.subject && metadata.subject !== '') {
      groupName = metadata.subject;
      memberCount = (metadata.participants || []).length;
    }
  } catch {}

  // NEVER send "WhatsApp Group" to the backend — it overwrites real names.
  // If we couldn't get the name, just register the group JID without a name
  // update and let refreshGroupNames handle it later.
  if (!groupName) {
    if (!syncedGroups.has(groupJid)) {
      // First time seeing this group — register it with a placeholder
      // but the backend will only store it if it doesn't already exist
      try {
        await fetch(`${BACKEND_URL}/api/bot/sync-group`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ wa_id: groupJid, name: '', member_count: 0 }),
        });
        syncedGroups.add(groupJid);
      } catch {}
    }
    return;
  }

  try {
    await fetch(`${BACKEND_URL}/api/bot/sync-group`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ wa_id: groupJid, name: groupName, member_count: memberCount }),
    });
    syncedGroups.add(groupJid);
    console.log(`  Synced group: ${groupName} (${groupJid})`);
  } catch (err) {
    console.error(`  Group sync failed for ${groupJid}: ${err.message}`);
  }
}

// Refresh group names from WhatsApp metadata for all known groups
async function refreshGroupNames(sock) {
  const groups = [...syncedGroups];
  for (let i = 0; i < groups.length; i++) {
    const groupJid = groups[i];
    try {
      const metadata = await sock.groupMetadata(groupJid);
      const name = metadata?.subject;
      if (name && name !== 'WhatsApp Group' && name !== '') {
        console.log(`  [GROUP-NAME] ${groupJid} → "${name}"`);
        await fetch(`${BACKEND_URL}/api/bot/sync-group`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ wa_id: groupJid, name, member_count: (metadata?.participants || []).length }),
        });
      }
    } catch (err) {
      console.log(`  [GROUP-NAME] Failed for ${groupJid}: ${err.message}`);
    }
    // Delay between groups to avoid 440 disconnects
    if (i < groups.length - 1) await new Promise(r => setTimeout(r, 2000));
  }
  console.log(`  Refreshed names for ${groups.length} groups.`);
}

async function preloadSyncedGroups() {
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/enabled-groups`);
    const data = await res.json();
    // All groups that exist in the backend are considered "synced"
    // (they were initialized previously via /reyna init)
    const res2 = await fetch(`${BACKEND_URL}/api/groups/settings`, {
      headers: { 'Content-Type': 'application/json' },
    });
    // This endpoint requires auth, so fall back to enabled-groups
    for (const gid of (data.groups || [])) {
      syncedGroups.add(gid);
    }
    // Also load all known group WA IDs from the bot/sync-group registrations
    const res3 = await fetch(`${BACKEND_URL}/api/bot/known-groups`);
    const data3 = await res3.json();
    for (const gid of (data3.groups || [])) {
      syncedGroups.add(gid);
    }
    console.log(`  Pre-loaded ${syncedGroups.size} initialized groups.`);
  } catch (err) {
    console.error('  Pre-load groups failed:', err.message);
  }
}

// ─── DM Handler — private chat, personal assistant mode ───

async function handleDM(sock, msg) {
  const chat = msg.key.remoteJid; // user's @s.whatsapp.net OR @lid JID
  let senderPhone = phoneFromJid(chat);
  const pushName = msg.pushName || '';
  let rawText = (msg.message?.conversation || msg.message?.extendedTextMessage?.text || '').trim();

  if (!rawText) return; // ignore non-text DMs (stickers, status etc.)
  // Defensive: skip our own bot echoes that survived the top-level filter
  if (isBotEcho(rawText)) return;

  // Wake-word gate (with session continuation) — Reyna requires the "reyna"
  // wake word to START a conversation, but for the next 10 minutes after a
  // reply, the same sender can send follow-ups WITHOUT the wake word. This
  // makes "1" / "second one" / "in simpler words" / "tell me more" work
  // naturally without spamming strangers who randomly DM the linked number.
  let text = rawText;
  if (hasWakeWord(rawText)) {
    text = stripWakeWord(rawText);
  } else if (!isSessionActive(chat)) {
    return; // no wake word, no active session → silent ignore
  }
  if (!text) {
    // Bare "reyna" with no content — send a friendly greeting so the user
    // knows the bot is reachable.
    await dmSend(sock, chat, {
      text: '👋 Hey! Ask me anything about your shared notes — I can find files, summarise them, or answer questions.\n\nExample: *reyna find notes by srikar* or *reyna explain wien bridge oscillator*',
    });
    return;
  }

  // For @lid (hidden number) JIDs, phoneFromJid may return garbage. Fall back
  // to using the JID itself as the identity key — backend will treat it as
  // an opaque user phone and resolve groups via the phone-fallback path.
  if (!isRealPhoneNumber(senderPhone)) {
    senderPhone = chat.split('@')[0]; // use raw JID-local part as the key
    console.log(`  [DM] non-phone JID, using "${senderPhone}" as key`);
  }

  console.log(`  [DM] ${pushName || senderPhone}: ${text}`);

  // ── Visible "thinking" indicator ──
  // Send a real text message (not just a reaction) so the user clearly sees
  // Reyna is processing. The thinking message stays in the chat — no edits
  // or deletions, since Baileys' edit/delete reliability is hit-or-miss.
  await dmSend(sock, chat, { text: '🔍 _looking through your notes..._' });

  // Use the BOT OWNER'S phone for backend group-resolution. The DM sender
  // (a friend) almost certainly isn't a member of the dashboard owner's
  // groups, so retrieval scoped by the friend's phone returns nothing.
  // dmContext / follow-ups stay keyed by sender so each conversation is
  // independent.
  const backendPhone = botOwnerPhone || senderPhone;

  // ── 1. Follow-up: pick a file by index from previous results ──
  const prev = getDmContext(senderPhone);
  const followIdx = detectFollowupIndex(text);
  if (prev && prev.lastFiles && prev.lastFiles.length > 0 && followIdx >= 0 && followIdx < prev.lastFiles.length) {
    const f = prev.lastFiles[followIdx];
    const url = driveViewUrl(f.drive_file_id);
    if (url) {
      await dmSend(sock, chat, {
        text: `📎 *${f.file_name}*${f.shared_by_name ? ` — shared by ${f.shared_by_name}` : ''}\n${url}\n\n_ask me anything about this file — I can summarise, explain, or quote from it._`,
      });
    } else {
      await dmSend(sock, chat, {
        text: `Found *${f.file_name}* but it's not pushed to Drive yet — check your dashboard staging area.`,
      });
    }
    // Remember the focused file so subsequent free-form questions Q&A about it
    setDmContext(senderPhone, {
      ...prev,
      lastFile: f,
      lastIntent: 'retrieve',
      ts: Date.now(),
    });
    markSessionActive(chat);
    return;
  }

  // ── 2. Q&A intent detection (with looser follow-up support) ──
  // ANY message that looks like a question routes to Q&A — and if there's
  // a recent prev turn (Q&A or retrieve), we thread it through as context
  // so follow-ups about the previously dropped file work naturally.
  const qaIntent = looksLikeQA(text);
  const hasFreshContext = prev && (Date.now() - prev.ts) < 30 * 60 * 1000;
  const isFollowupTurn = hasFreshContext && looksLikeFollowup(text);
  if (qaIntent || isFollowupTurn) {
    let prevTurn = null;
    if (hasFreshContext && (isFollowupTurn || qaIntent)) {
      // Build the prev-turn context from whichever shape the last interaction
      // produced — Q&A turn (lastQA) OR retrieve turn (lastFile, lastReply).
      if (prev.lastQA && prev.lastQA.question) {
        prevTurn = {
          question: prev.lastQA.question,
          answer: prev.lastQA.answer,
          sources: prev.lastQA.sources || [],
        };
      } else if (prev.lastFile) {
        // Synthesize a "previous turn" out of the file we just dropped so
        // Gemini knows the user is asking about THAT file.
        prevTurn = {
          question: prev.lastQuery || `looking at ${prev.lastFile.file_name}`,
          answer: `I shared the file *${prev.lastFile.file_name}*${prev.lastFile.shared_by_name ? ` (by ${prev.lastFile.shared_by_name})` : ''}.`,
          sources: [prev.lastFile.file_name],
        };
      } else if (prev.lastQuery) {
        prevTurn = {
          question: prev.lastQuery,
          answer: prev.lastReply || '',
          sources: [],
        };
      }
      if (prevTurn) console.log(`  [DM] Q&A with prev context — prev question="${prevTurn.question}"`);
    }
    const resp = await notesQA('', backendPhone, text, prevTurn);
    let answer = resp.answer || "I couldn't find anything to answer that.";
    if (resp.sources && resp.sources.length > 0) {
      answer += `\n\n📎 _sources: ${resp.sources.slice(0, 3).join(', ')}_`;
    }
    await dmSend(sock, chat, { text: answer });
    setDmContext(senderPhone, {
      lastQuery: text,
      lastFiles: [],
      lastFile: prev?.lastFile || null, // keep file context across Q&A turns
      lastReply: answer,
      lastIntent: 'qa',
      lastQA: { question: text, answer: resp.answer || '', sources: resp.sources || [] },
    });
    markSessionActive(chat);
    return;
  }

  // ── 3. Default: NLP retrieval ──
  const resp = await nlpRetrieve('', backendPhone, text);
  const files = resp.files || [];
  const driveMatches = resp.drive_matches || [];

  let reply = resp.reply || '';
  let focusedFile = null;

  if (files.length === 1) {
    // Exactly one match → drop the link immediately
    const f = files[0];
    focusedFile = f;
    const url = driveViewUrl(f.drive_file_id);
    if (url) {
      reply += `\n\n📎 *${f.file_name}*\n${url}\n\n_ask me anything about this file — I can summarise, explain, or quote from it._`;
    } else {
      reply += `\n\n📎 *${f.file_name}* (still in staging — check your dashboard)`;
    }
  } else if (files.length > 1) {
    reply += `\n\n${formatFilesForReply(files, 5)}\n\n_reply with a number (e.g. "1") to get the link, or ask a question about any of these._`;
  } else if (driveMatches.length > 0) {
    const lines = driveMatches.slice(0, 5).map((m, i) => {
      const url = driveViewUrl(m.file_id);
      return `${i + 1}. *${m.file_name}* — in ${m.folder_name}/${url ? '\n   ' + url : ''}`;
    });
    reply += `\n\n${lines.join('\n')}`;
  }

  if (!reply) reply = "I couldn't find anything matching that. Try rephrasing or being more specific.";

  await dmSend(sock, chat, { text: reply });

  // Cache files + focused file for follow-up resolution
  setDmContext(senderPhone, {
    lastQuery: text,
    lastFiles: files,
    lastFile: focusedFile,
    lastReply: reply,
    lastIntent: 'retrieve',
  });
  markSessionActive(chat);
}

// looksLikeFollowup returns true if a message reads like a refinement of a
// previous Q&A turn rather than a fresh question. Used by the DM handler to
// decide whether to thread the conversation through to the backend.
function looksLikeFollowup(text) {
  const t = text.toLowerCase().trim();
  if (t.length < 60) {
    // short messages after a Q&A are usually refinements
    const cues = [
      'tell me more', 'more', 'elaborate', 'expand', 'continue', 'go on',
      'what about', 'and ', 'also ', 'how about', 'aur ', 'aur kya',
      'what else', 'anything else', 'kuch aur', 'aage', 'next',
      'simpler', 'simply', 'simpler words', 'in simple', 'asaan', 'easy',
      'example', 'examples', 'for example', 'udaharan',
      'why', 'kyu', 'kyun', 'how', 'kaise',
      'shorter', 'shorten', 'short me', 'in short', 'briefly',
      'longer', 'detail', 'in detail', 'detailed', 'vistaar',
      'translate', 'in english', 'in hindi',
      'that ', 'this ', 'it ', // pronoun-led short followups
    ];
    for (const c of cues) if (t.startsWith(c) || t.includes(' ' + c) || t === c.trim()) return true;
  }
  return false;
}

// looksLikeQA returns true for messages that read like questions about notes
// content rather than simple file lookups.
function looksLikeQA(text) {
  const t = text.toLowerCase();
  const cues = [
    'explain', 'samjhao', 'samjhaa', 'batao', 'kya hai', 'what is', 'what are',
    'how does', 'how do', 'why does', 'why is', 'why are', 'define', 'definition',
    'summarize', 'summary', 'describe', 'tell me about', 'difference between',
    'compare', 'derive', 'derivation', 'prove', 'theorem',
  ];
  for (const c of cues) {
    if (t.includes(c)) return true;
  }
  return false;
}

// ─── Message Handler ───

async function handleMessage(sock, msg) {
  const chat = msg.key.remoteJid;
  if (!chat) return;

  const isGroup = chat.endsWith('@g.us');

  // ── 1:1 DM branch — Reyna acts as a personal assistant ──
  // Catches both @s.whatsapp.net (canonical phone JIDs) and @lid (hidden /
  // PN-protected accounts that WhatsApp now uses for many DMs).
  // We allow fromMe in DMs so the linked user can DM their own bot, but
  // first we skip self-echoes by checking the BOT_MARK prefix on the text.
  if (!isGroup) {
    const incoming = msg.message?.conversation || msg.message?.extendedTextMessage?.text || '';
    if (msg.key.fromMe && isBotEcho(incoming)) return; // our own reply, ignore
    return handleDM(sock, msg);
  }

  // Check for /reyna init BEFORE the fromMe filter — the linked user
  // (whose messages are fromMe) needs to be able to initialize groups.
  const initText = msg.message?.conversation || msg.message?.extendedTextMessage?.text || '';
  const isInit = initText.trim().toLowerCase() === '/reyna init';
  if (isInit) {
    console.log(`  [INIT] detected /reyna init from ${msg.key.participant || chat} fromMe=${msg.key.fromMe}`);
  } else if (msg.key.fromMe) {
    // For all other group messages, skip fromMe (bot's own replies)
    return;
  }

  const sender = msg.key.participant || chat;
  let senderPhone = phoneFromJid(sender);
  let pushName = msg.pushName || '';
  const messageType = getContentType(msg.message);

  // If pushName is empty OR phone is a LID (not real), try to resolve from group metadata
  if ((!pushName || !isRealPhoneNumber(senderPhone)) && chat.endsWith('@g.us')) {
    try {
      const metadata = await sock.groupMetadata(chat);
      const participants = metadata?.participants || [];

      // Try exact match first, then partial match on the number portion
      const senderNum = sender.split('@')[0].split(':')[0];
      let participant = participants.find(p => p.id === sender);
      if (!participant) {
        participant = participants.find(p => p.id?.split(':')[0] === senderNum);
      }
      if (!participant) {
        // Try matching by LID — some participants have lid field
        participant = participants.find(p => p.lid?.split(':')[0] === senderNum || p.lid === sender);
      }

      if (participant) {
        if (!pushName) {
          pushName = participant.notify || participant.name || '';
        }
        if (!isRealPhoneNumber(senderPhone)) {
          // Try participant.id first
          const participantPhone = phoneFromJid(participant.id);
          if (isRealPhoneNumber(participantPhone)) {
            senderPhone = participantPhone;
          }
        }
      }

      // Last resort: if we still don't have a real phone, scan ALL participants
      // to find anyone whose LID matches our sender
      if (!isRealPhoneNumber(senderPhone)) {
        for (const p of participants) {
          if (p.lid && p.lid.split(':')[0] === senderNum) {
            const pPhone = phoneFromJid(p.id);
            if (isRealPhoneNumber(pPhone)) {
              senderPhone = pPhone;
              if (!pushName) pushName = p.notify || p.name || '';
              break;
            }
          }
        }
      }
    } catch (err) {
      console.log(`  [LID-RESOLVE] Failed for ${sender}: ${err.message}`);
    }
  }

  // Log raw sender info for debugging
  if (messageType === 'documentMessage' || messageType === 'documentWithCaptionMessage') {
    console.log(`  [DEBUG] sender_jid=${sender} phone=${senderPhone} pushName=${pushName} isReal=${isRealPhoneNumber(senderPhone)}`);
  }

  // ── Handle /reyna init — works in ANY group, registers it ──
  const text = msg.message?.conversation || msg.message?.extendedTextMessage?.text || '';
  if (isInit || text.trim().toLowerCase() === '/reyna init') {
    console.log(`  [INIT] ${pushName}: /reyna init in ${chat}`);

    // Send the confirmation message FIRST, before any heavy API calls.
    // The 440 disconnects happen because groupMetadata + backend sync +
    // refreshEnabledGroups all fire rapidly and WhatsApp kills the connection
    // before sendMessage gets a chance to run.
    try {
      await sock.sendMessage(chat, {
        text: '*Reyna:* Initialized! This group is now being monitored.\n\nHow to use:\n• Share a file and it gets auto-saved (or react 📌 in reaction mode)\n• DM me for search and Q&A\n• "reyna save" / "reyna push" / "reyna status"\n• "/reyna stop" to disable tracking\n\nManage settings on your dashboard.',
      });
      console.log(`  [INIT] sent init message to ${chat}`);
    } catch (sendErr) {
      console.error(`  [INIT] failed to send init message: ${sendErr.message}`);
    }

    // Sync the group name + explicitly enable it in the backend.
    // ensureGroupSynced registers the group, then we call the command
    // endpoint with "/reyna enable" to force-enable the group settings.
    syncedGroups.delete(chat);
    await ensureGroupSynced(sock, chat);
    try {
      await fetch(`${BACKEND_URL}/api/bot/command`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          group_wa_id: chat, command: '/reyna enable',
          user_phone: senderPhone, user_name: pushName,
        }),
      });
    } catch {}
    refreshEnabledGroups().catch(() => {});
    return;
  }

  // ── Handle /reyna stop — disable tracking from WhatsApp ──
  if (text.trim().toLowerCase() === '/reyna stop') {
    try {
      // Find group ID and disable it
      const res = await fetch(`${BACKEND_URL}/api/bot/command`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          group_wa_id: chat, command: '/reyna disable',
          user_phone: senderPhone, user_name: pushName,
        }),
      });
      syncedGroups.delete(chat);
      enabledGroups.delete(chat);
      await sock.sendMessage(chat, {
        text: '*Reyna:* Tracking disabled for this group. Files will no longer be captured.\n\nTo re-enable, send /reyna init.',
      });
      console.log(`  [STOP] disabled tracking for ${chat}`);
    } catch (err) {
      console.error(`  [STOP] failed: ${err.message}`);
    }
    return;
  }

  // ── Only process messages from initialized groups ──
  if (!syncedGroups.has(chat)) return;

  // Re-sync group name in background — but throttled to once per group
  // per 10 minutes to avoid 440 disconnects from too many groupMetadata calls.
  const lastSync = groupLastSyncTime.get(chat) || 0;
  if (Date.now() - lastSync > 10 * 60 * 1000) {
    groupLastSyncTime.set(chat, Date.now());
    ensureGroupSynced(sock, chat).catch(() => {});
  }

  const { mode, enabled } = await getGroupMode(chat);
  console.log(`  [MODE] ${chat.slice(0,12)}... enabled=${enabled} mode=${mode}`);

  // If the group is disabled (toggle off in dashboard or /reyna stop), skip everything
  if (!enabled) {
    console.log(`  [SKIP] group disabled, ignoring message`);
    return;
  }

  // ── Track document shares ──
  if (messageType === 'documentMessage' || messageType === 'documentWithCaptionMessage') {
    const doc = msg.message.documentMessage ||
      msg.message?.documentWithCaptionMessage?.message?.documentMessage;
    if (doc) {
      try {
        const buffer = await downloadMediaMessage(msg, 'buffer', {}, {
          logger, reuploadRequest: sock.updateMediaMessage,
        });
        const fileName = doc.fileName || 'file';
        const msgId = msg.key.id;
        trackFile(chat, msgId, {
          fileName, buffer,
          mimeType: doc.mimetype || 'application/octet-stream',
          fileSize: Number(doc.fileLength || buffer.length),
          sender: senderPhone, senderName: pushName,
          ts: Date.now(),
        });

        const untracked = getUntrackedFiles(chat);
        console.log(`  Tracked: ${fileName} (${(buffer.length / 1024).toFixed(0)}KB) from ${pushName} | mode=${mode} | ${untracked.length} pending`);

        // AUTO MODE: stage immediately and silently
        if (mode === 'auto') {
          const resp = await uploadFile(chat, senderPhone, pushName, {
            fileName, fileSize: Number(doc.fileLength || buffer.length),
            mimeType: doc.mimetype || 'application/octet-stream',
          }, buffer);
          markStaged(chat, msgId);

          // Also stage any OTHER untracked files (handles mode switch from reaction→auto)
          const stillUntracked = getUntrackedFiles(chat);
          for (const uf of stillUntracked) {
            if (uf.buffer && uf.buffer.length > 0) {
              console.log(`  Auto-staging previously untracked: ${uf.fileName}`);
              await uploadFile(chat, uf.sender || senderPhone, uf.senderName || pushName, {
                fileName: uf.fileName, fileSize: uf.fileSize,
                mimeType: uf.mimeType,
              }, uf.buffer);
              markStaged(chat, uf.msgId);
            }
          }

          // React with checkmark — retry once after a short delay if it fails
          // (WhatsApp throttles rapid-fire reactions when multiple files land at once)
          for (let attempt = 0; attempt < 2; attempt++) {
            try {
              await sock.sendMessage(chat, { react: { text: '✅', key: msg.key } });
              break;
            } catch (reactErr) {
              if (attempt === 0) {
                await new Promise(r => setTimeout(r, 1000 + Math.random() * 1000));
              } else {
                console.log(`  [REACT] failed for ${fileName}: ${reactErr.message}`);
              }
            }
          }
          console.log(`  Auto-staged: ${fileName}`);
          return;
        }

        // REACTION MODE: do nothing, wait for 📌 reaction
      } catch (err) {
        console.error('Download failed:', err.message);
      }
    }
    return;
  }

  // ── Text messages — check for Reyna commands ──
  if (!text.trim()) return;

  if (!isReynaMessage(text)) return;

  console.log(`  ${pushName}: ${text}`);

  // Legacy slash command support
  if (text.trim().toLowerCase().startsWith('/reyna')) {
    const resp = await sendCommand(chat, text, senderPhone, pushName);
    await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
    return;
  }

  // Natural language intent detection
  const { intent, query } = detectIntent(text);

  switch (intent) {
    case 'save': {
      const untracked = getUntrackedFiles(chat);
      if (untracked.length === 0) {
        const last = getLastFile(chat);
        if (!last || last.staged) {
          await sock.sendMessage(chat, { text: '*Reyna:* No files to stage. Share a document first.' });
          return;
        }
        const resp = await uploadFile(chat, senderPhone, pushName, {
          fileName: last.fileName, fileSize: last.fileSize, mimeType: last.mimeType,
        }, last.buffer);
        markStaged(chat, last.msgId);
        await sock.sendMessage(chat, { text: `*Reyna:* Staged \`${last.fileName}\`. Say "reyna push" to commit to Drive.` });
        return;
      }
      // Stage all untracked
      let staged = 0;
      const names = [];
      for (const f of untracked) {
        await uploadFile(chat, senderPhone, pushName, {
          fileName: f.fileName, fileSize: f.fileSize, mimeType: f.mimeType,
        }, f.buffer);
        markStaged(chat, f.msgId);
        staged++;
        names.push(f.fileName);
      }
      let reply = `*Reyna:* ${staged} file(s) staged.`;
      if (staged <= 3) reply += ' ' + names.join(', ');
      reply += '\nSay "reyna push" to commit, or manage on your dashboard.';
      await sock.sendMessage(chat, { text: reply });
      return;
    }

    case 'push': {
      const resp = await sendCommand(chat, '/reyna commit', senderPhone, pushName);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
      return;
    }

    case 'history': {
      const resp = await sendCommand(chat, '/reyna log', senderPhone, pushName);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
      return;
    }

    case 'status': {
      const resp = await sendCommand(chat, '/reyna status', senderPhone, pushName);
      const untracked = getUntrackedFiles(chat);
      let statusMsg = resp.reply || '';
      if (untracked.length > 0) {
        statusMsg += `\n\nUntracked: ${untracked.length} file(s). React with 📌 or say "reyna save".`;
      }
      await sock.sendMessage(chat, { text: `*Reyna:* ${statusMsg}` });
      return;
    }

    case 'help': {
      await sock.sendMessage(chat, {
        text: '*Reyna:* In groups I handle file capture only — share files (or react 📌), and say "reyna save / push / status".\n\n💬 For search, retrieval, and Q&A, DM me directly. No wake word needed there.',
      });
      return;
    }

    case 'remove': {
      const resp = await sendCommand(chat, `/reyna rm ${query}`, senderPhone, pushName);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
      return;
    }

    // Search / NLP retrieval / Q&A are intentionally NOT handled in groups —
    // they would clutter group chat. Tell the user to DM Reyna instead.
    case 'search':
    case 'nlp_retrieve':
    case 'qa': {
      await sock.sendMessage(chat, {
        text: '*Reyna:* For search and Q&A, DM me directly — keeps the group clean. 💬',
      });
      return;
    }

    default: {
      await sock.sendMessage(chat, {
        text: '*Reyna:* I only handle file capture in groups (share files, react 📌, "reyna save / push / status"). For search and Q&A, DM me directly. 💬',
      });
      return;
    }
  }
}

// ─── Reaction Handler (📌 to stage) ───

async function handleReaction(sock, reaction) {
  const chat = reaction.key.remoteJid;
  if (!chat?.endsWith('@g.us')) return;
  if (!syncedGroups.has(chat)) return;

  const emoji = reaction.reaction?.text;
  if (!emoji || emoji === '') return; // Empty = reaction removed

  // Check if this is a staging reaction (📌 by default, configurable)
  const validEmojis = ['📌', '⭐', '🔖', '💾'];
  if (!validEmojis.includes(emoji)) return;

  const reactedMsgId = reaction.key.id;
  const reactor = reaction.key.participant || chat;
  const reactorPhone = phoneFromJid(reactor);
  const reactorName = reaction.pushName || '';

  // Find the tracked file matching this message ID
  const tracker = getGroupTracker(chat);
  const fileInfo = tracker.get(reactedMsgId);

  if (!fileInfo) {
    console.log(`  Reaction ${emoji} on unknown msg ${reactedMsgId}`);
    return;
  }

  if (fileInfo.staged) {
    console.log(`  ${fileInfo.fileName} already staged, ignoring reaction`);
    return;
  }

  console.log(`  ${emoji} reaction on ${fileInfo.fileName} by ${reactorName}`);

  // Upload file to backend (this stages it)
  const resp = await uploadFile(chat, reactorPhone, reactorName, {
    fileName: fileInfo.fileName,
    fileSize: fileInfo.fileSize,
    mimeType: fileInfo.mimeType,
  }, fileInfo.buffer);

  markStaged(chat, reactedMsgId);

  // React with ✅ to confirm (minimal group footprint — no text message)
  try {
    await sock.sendMessage(chat, {
      react: { text: '✅', key: { remoteJid: chat, id: reactedMsgId, participant: fileInfo.sender + '@s.whatsapp.net' } },
    });
  } catch (err) {
    // Fallback: can't react to the original message, just log
    console.log(`  Could not react: ${err.message}`);
  }

  console.log(`  Staged via reaction: ${fileInfo.fileName}`);
}

// ─── Start ───

async function startBot() {
  const { state, saveCreds } = await useMultiFileAuthState(AUTH_DIR);
  const { version } = await fetchLatestBaileysVersion();
  console.log(`  WA Web version: ${version.join('.')}`);

  const sock = makeWASocket({
    version,
    auth: { creds: state.creds, keys: makeCacheableSignalKeyStore(state.keys, logger) },
    logger, browser: ['Reyna', 'Chrome', '4.0.0'],
    syncFullHistory: false, markOnlineOnConnect: false,
  });

  sock.ev.on('connection.update', (update) => {
    const { connection, lastDisconnect, qr } = update;
    if (qr) {
      console.log('\n  Scan with WhatsApp → Linked Devices → Link a Device:\n');
      qrcode.generate(qr, { small: true });
    }
    if (connection === 'open') {
      // Capture the linked WhatsApp account's phone number — used as the
      // owner phone for all DM-mode backend calls (retrieval, Q&A) so the
      // search is scoped to the dashboard owner's groups, not the friend's.
      try {
        const ownerJid = sock.user?.id || '';
        const ownerPhone = phoneFromJid(ownerJid);
        if (isRealPhoneNumber(ownerPhone)) {
          setBotOwnerPhone(ownerPhone);
        } else {
          console.log(`  Could not resolve bot owner phone from JID ${ownerJid}`);
        }
      } catch (e) {
        console.log(`  Bot owner phone resolution failed: ${e.message}`);
      }
      console.log('\n  ┌──────────────────────────────────────────┐');
      console.log('  │  Reyna Bot — Connected                   │');
      console.log('  │                                          │');
      console.log('  │  Natural language:                        │');
      console.log('  │    "reyna find DSA notes"                 │');
      console.log('  │    "reyna save" / "reyna push"            │');
      console.log('  │    "reyna status" / "reyna help"          │');
      console.log('  │                                          │');
      console.log('  │  Emoji tracking:                          │');
      console.log('  │    React 📌 on any file to stage it       │');
      console.log('  │                                          │');
      console.log('  │  Auto-commit: 24 hours                    │');
      console.log('  └──────────────────────────────────────────┘\n');

      // Fetch enabled groups and pre-load synced groups
      refreshEnabledGroups();
      preloadSyncedGroups().then(() => refreshGroupNames(sock));
      // Refresh every 5 minutes
      setInterval(refreshEnabledGroups, 5 * 60 * 1000);
      // Refresh group names every 30 minutes
      // Don't refresh names on an interval — it causes too many groupMetadata
      // calls which trigger 440 disconnects. Names are synced on-demand when
      // messages arrive (throttled to once per 10 min per group).
    }
    if (connection === 'close') {
      const code = lastDisconnect?.error?.output?.statusCode;
      console.log(`  Disconnected (${code})`);
      if ([401, 403, 405].includes(code)) {
        fs.rmSync(AUTH_DIR, { recursive: true, force: true });
        setTimeout(startBot, 2000);
      } else { setTimeout(startBot, 5000); }
    }
  });

  sock.ev.on('creds.update', saveCreds);

  // ── Message handler ──
  // Process messages sequentially with a small gap between each one.
  // When 6 files land in one batch, this spaces out the downloads +
  // uploads + reactions so WhatsApp doesn't throttle the reactions.
  sock.ev.on('messages.upsert', async ({ messages }) => {
    for (let i = 0; i < messages.length; i++) {
      try {
        await handleMessage(sock, messages[i]);
      } catch (err) {
        console.error('Error:', err.message);
      }
      // Small gap between messages in a batch to avoid WhatsApp throttling
      if (messages.length > 1 && i < messages.length - 1) {
        await new Promise(r => setTimeout(r, 800));
      }
    }
  });

  // ── Reaction handler (📌 staging) ──
  sock.ev.on('messages.reaction', async (reactions) => {
    for (const r of reactions) {
      try { await handleReaction(sock, r); } catch (err) { console.error('Reaction error:', err.message); }
    }
  });
}

console.log('\n  Reyna WhatsApp Bot starting...');
console.log(`  Backend: ${BACKEND_URL}`);

if (fs.existsSync(path.join(AUTH_DIR, 'creds.json'))) {
  try { JSON.parse(fs.readFileSync(path.join(AUTH_DIR, 'creds.json'), 'utf8')); }
  catch { console.log('  Cleaning stale auth...'); fs.rmSync(AUTH_DIR, { recursive: true, force: true }); }
}

startBot().catch(err => { console.error('Fatal:', err); process.exit(1); });
