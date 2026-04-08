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

async function notesQA(groupJid, userPhone, question) {
  try {
    const res = await fetch(`${BACKEND_URL}/api/nlp/qa`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question, group_wa_id: groupJid, user_phone: userPhone }),
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

async function getGroupMode(groupJid) {
  // Always fetch fresh — dashboard mode changes must take effect immediately
  try {
    const res = await fetch(`${BACKEND_URL}/api/bot/group-mode?wa_id=${encodeURIComponent(groupJid)}`);
    const data = await res.json();
    return data.mode || 'auto';
  } catch {
    return 'auto';
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

  // SEARCH / FIND
  for (const p of ['find', 'search', 'look for', 'where is', 'get me', 'show me', 'do you have', 'send me', 'need']) {
    if (stripped.includes(p)) {
      const idx = stripped.indexOf(p);
      let query = stripped.slice(idx + p.length).trim().replace(/["'?.,!]/g, '');
      return { intent: 'search', query: query || stripped };
    }
  }

  // Bare academic terms → search
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

  // Q&A patterns — questions about content of notes
  const qaPatterns = ['summarize', 'explain', 'what does', 'what is', 'tell me about',
    'what did the teacher', 'what are the', 'describe', 'define',
    'how does', 'why does', 'compare', 'difference between',
    'what chapter', 'from our notes', 'from the notes', 'according to'];
  for (const p of qaPatterns) {
    if (stripped.includes(p)) return { intent: 'qa', query: stripped };
  }

  // NLP retrieval patterns — questions about WHO shared WHAT WHEN
  const nlpPatterns = ['what did', 'who shared', 'who uploaded', 'has anyone shared',
    'do we have', 'is there', "what's new", 'anything about', 'any files about',
    'files from', 'shared by'];
  for (const p of nlpPatterns) {
    if (stripped.includes(p)) return { intent: 'nlp_retrieve', query: stripped };
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
  if (syncedGroups.has(groupJid)) return;
  let groupName = 'WhatsApp Group';
  let memberCount = 0;

  // Try multiple times to get metadata (sometimes fails on first connect)
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const metadata = await sock.groupMetadata(groupJid);
      console.log(`  [SYNC] Attempt ${attempt + 1}: subject="${metadata?.subject}" participants=${metadata?.participants?.length}`);
      if (metadata?.subject && metadata.subject !== '') {
        groupName = metadata.subject;
        memberCount = (metadata.participants || []).length;
        break;
      }
    } catch (err) {
      console.log(`  [SYNC] Attempt ${attempt + 1} failed: ${err.message}`);
      if (attempt < 2) await new Promise(r => setTimeout(r, 1000)); // wait 1s before retry
    }
  }

  try {
    await fetch(`${BACKEND_URL}/api/bot/sync-group`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        wa_id: groupJid,
        name: groupName,
        member_count: memberCount,
      }),
    });
    syncedGroups.add(groupJid);
    console.log(`  Synced group: ${groupName} (${groupJid})`);
  } catch (err) {
    console.error(`  Group sync failed for ${groupJid}: ${err.message}`);
  }
}

// Refresh group names from WhatsApp metadata for all known groups
async function refreshGroupNames(sock) {
  for (const groupJid of syncedGroups) {
    try {
      const metadata = await sock.groupMetadata(groupJid);
      const name = metadata?.subject;
      console.log(`  [GROUP-NAME] ${groupJid} → "${name}"`);
      if (name && name !== 'WhatsApp Group') {
        await fetch(`${BACKEND_URL}/api/bot/sync-group`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            wa_id: groupJid,
            name,
            member_count: (metadata?.participants || []).length,
          }),
        });
        console.log(`  [GROUP-NAME] Updated: ${groupJid} → "${name}"`);
      }
    } catch (err) {
      console.log(`  [GROUP-NAME] Failed for ${groupJid}: ${err.message}`);
    }
  }
  console.log(`  Refreshed names for ${syncedGroups.size} groups.`);
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

// ─── Message Handler ───

async function handleMessage(sock, msg) {
  const chat = msg.key.remoteJid;
  if (!chat?.endsWith('@g.us')) return;
  if (msg.key.fromMe) return;

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
  if (text.trim().toLowerCase() === '/reyna init') {
    console.log(`  ${pushName}: /reyna init in ${chat}`);
    syncedGroups.delete(chat); // Force re-sync to update name from WhatsApp metadata
    await ensureGroupSynced(sock, chat);
    // Refresh enabled groups so this group is now recognized
    await refreshEnabledGroups();
    try {
      await fetch(`${BACKEND_URL}/api/bot/command`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          group_wa_id: chat, command: '/reyna help',
          user_phone: senderPhone, user_name: pushName,
        }),
      });
    } catch {}
    await sock.sendMessage(chat, {
      text: '*Reyna:* Initialized! This group is now being monitored.\n\nHow to use:\n• Share a file → auto-saved (or react 📌 in reaction mode)\n• "reyna find [topic]" → search files\n• "reyna push" → commit to Drive now\n• "reyna status" → see what\'s new\n\nManage settings on your dashboard.',
    });
    return;
  }

  // ── Only process messages from initialized groups ──
  if (!syncedGroups.has(chat)) return;

  const mode = await getGroupMode(chat);

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

          // React with checkmark instead of sending a message (minimal footprint)
          try {
            await sock.sendMessage(chat, { react: { text: '✅', key: msg.key } });
          } catch {}
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

    case 'search': {
      const resp = await sendCommand(chat, `/reyna find ${query}`, senderPhone, pushName);
      // Brief response in group (search results benefit everyone)
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
      const resp = await sendCommand(chat, '/reyna help', senderPhone, pushName);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
      return;
    }

    case 'remove': {
      const resp = await sendCommand(chat, `/reyna rm ${query}`, senderPhone, pushName);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply}` });
      return;
    }

    case 'nlp_retrieve': {
      const resp = await nlpRetrieve(chat, senderPhone, query);
      await sock.sendMessage(chat, { text: `*Reyna:* ${resp.reply || 'No results found.'}` });
      return;
    }

    case 'qa': {
      // Show typing indicator
      await sock.sendMessage(chat, { text: '*Reyna:* 🔍 Searching your notes...' });
      const resp = await notesQA(chat, senderPhone, query);
      let qaReply = resp.answer || 'Could not find an answer.';
      if (resp.sources && resp.sources.length > 0) {
        qaReply += '\n\n📎 Sources: ' + resp.sources.join(', ');
      }
      await sock.sendMessage(chat, { text: `*Reyna:* ${qaReply}` });
      return;
    }

    default: {
      // Try NLP retrieval as fallback for unrecognized queries
      const nlpResp = await nlpRetrieve(chat, senderPhone, text);
      if (nlpResp.files && nlpResp.files.length > 0) {
        await sock.sendMessage(chat, { text: `*Reyna:* ${nlpResp.reply}` });
      } else {
        await sock.sendMessage(chat, {
          text: '*Reyna:* I didn\'t understand that. Say "reyna help" to see what I can do.',
        });
      }
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
      setInterval(() => refreshGroupNames(sock), 30 * 60 * 1000);
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
  sock.ev.on('messages.upsert', async ({ messages }) => {
    for (const m of messages) {
      try { await handleMessage(sock, m); } catch (err) { console.error('Error:', err.message); }
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
