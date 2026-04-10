import { notify } from '../components/Notifications'

const API = '/api';

function getHeaders() {
  const h = { 'Content-Type': 'application/json' };
  const token = localStorage.getItem('reyna_token');
  if (token) h['Authorization'] = `Bearer ${token}`;
  return h;
}

async function req(path, opts = {}) {
  const isMutation = opts.method === 'POST' || opts.method === 'PUT' || opts.method === 'DELETE'
  if (isMutation) notify.showLoader()

  try {
    const res = await fetch(`${API}${path}`, { headers: getHeaders(), ...opts });
    if (res.status === 401) {
      localStorage.removeItem('reyna_token');
      localStorage.removeItem('reyna_user');
      window.location.href = '/login';
      return null;
    }
    const data = await res.json();
    // Only show error toasts for user-initiated mutations, not background polling GETs
    if (!res.ok && data?.error && isMutation) {
      notify.error(data.error)
    }
    return data;
  } catch (err) {
    // Only show network error toasts for mutations, not polling
    if (isMutation) {
      notify.error('Network error: ' + (err.message || 'Could not reach server'))
    }
    return null;
  } finally {
    if (isMutation) notify.hideLoader()
  }
}

export const api = {
  // Auth
  register: (phone, name) => req('/auth/register', { method: 'POST', body: JSON.stringify({ phone, name }) }),
  login: (phone) => req('/auth/login', { method: 'POST', body: JSON.stringify({ phone }) }),
  me: () => req('/me'),

  // Dashboard
  dashboard: () => req('/dashboard'),

  // Groups
  groups: () => req('/groups'),
  createGroup: (wa_id, name) => req('/groups', { method: 'POST', body: JSON.stringify({ wa_id, name }) }),

  // Files
  files: (groupId, limit, sortBy, sortOrder) => req(`/files?group_id=${groupId || ''}&limit=${limit || 50}&sort_by=${sortBy || ''}&sort_order=${sortOrder || ''}`),
  search: (q, groupId) => req(`/files/search?q=${encodeURIComponent(q)}&group_id=${groupId || ''}`),
  suggest: (q) => req(`/files/suggest?q=${encodeURIComponent(q)}`),
  versions: (fileId) => req(`/files/versions?file_id=${fileId}`),
  upload: (data) => req('/files/upload', { method: 'POST', body: JSON.stringify(data) }),
  deleteFile: (fileId) => req('/files/delete', { method: 'POST', body: JSON.stringify({ file_id: fileId }) }),
  deleteFiles: (fileIds) => req('/files/delete', { method: 'POST', body: JSON.stringify({ file_ids: fileIds }) }),
  removeStaged: (fileId) => req('/files/staged/remove', { method: 'POST', body: JSON.stringify({ file_id: fileId }) }),
  removeStagedAll: () => req('/files/staged/remove', { method: 'POST', body: JSON.stringify({ all: true }) }),
  removeStagedBatch: (fileIds) => req('/files/staged/remove', { method: 'POST', body: JSON.stringify({ file_ids: fileIds }) }),
  fileExists: (name, groupId) => req(`/files/exists?name=${encodeURIComponent(name)}&group_id=${groupId || ''}`),

  // File download/preview URL
  downloadUrl: (fileId) => `${API}/files/download?file_id=${fileId}`,
  getDownloadInfo: (fileId) => req(`/files/download?file_id=${fileId}`),

  // Activity
  activity: (groupId) => req(`/activity?group_id=${groupId}`),

  // Bot command
  botCommand: (groupWaId, command, userPhone) =>
    req('/bot/command', { method: 'POST', body: JSON.stringify({ group_wa_id: groupWaId, command, user_phone: userPhone }) }),

  // Waitlist
  joinWaitlist: (contact) => req('/waitlist', { method: 'POST', body: JSON.stringify({ contact }) }),
  waitlistCount: () => req('/waitlist'),

  // Health
  health: () => req('/health'),

  // Google Drive
  googleStatus: () => req('/auth/google/status'),
  googleConnect: () => req('/auth/google/connect'),
  googleDisconnect: () => req('/auth/google/disconnect', { method: 'POST' }),

  // Drive folders
  driveFolders: (parentId) => req(`/drive/folders?parent_id=${parentId || ''}`),
  driveTree: (parentId) => req(`/drive/tree?parent_id=${parentId || ''}`),
  driveRootFolders: () => req('/drive/root-folders'),
  changeDriveRoot: (folderId) => req('/drive/root', { method: 'POST', body: JSON.stringify({ folder_id: folderId }) }),
  createDriveFolder: (name, parentId, setAsRoot) => req('/drive/folder/create', { method: 'POST', body: JSON.stringify({ name, parent_id: parentId || '', set_as_root: !!setAsRoot }) }),
  renameDriveFolder: (folderId, newName) => req('/drive/folder/rename', { method: 'POST', body: JSON.stringify({ folder_id: folderId, new_name: newName }) }),
  deleteDriveFolder: (folderId) => req('/drive/folder/delete', { method: 'POST', body: JSON.stringify({ folder_id: folderId }) }),

  // v2 — Group Settings
  groupSettings: (groupId) => req(`/groups/settings?group_id=${groupId || ''}`),
  allGroupSettings: () => req('/groups/settings'),
  updateGroupSettings: (groupId, settings) => req('/groups/settings', { method: 'POST', body: JSON.stringify({ group_id: groupId, ...settings }) }),

  // v2 — Commit staged files to Drive
  commitStaged: () => req('/files/staged/commit', { method: 'POST' }),

  // v3 — NLP Conversational Retrieval
  nlpRetrieve: (query, groupWaId) => req('/nlp/retrieve', { method: 'POST', body: JSON.stringify({ query, group_wa_id: groupWaId || '' }) }),

  // v3 — Notes Q&A (with optional follow-up turn for multi-turn conversations)
  notesQA: (question, groupWaId, prev) => req('/nlp/qa', { method: 'POST', body: JSON.stringify({
    question,
    group_wa_id: groupWaId || '',
    previous_question: prev?.question || '',
    previous_answer: prev?.answer || '',
    previous_sources: prev?.sources || [],
  }) }),

  // v3 — LLM Status
  llmStatus: () => req('/llm/status'),
};

export function saveAuth(data) {
  if (data?.token) localStorage.setItem('reyna_token', data.token);
  if (data?.user) localStorage.setItem('reyna_user', JSON.stringify(data.user));
}

export function getUser() {
  try { return JSON.parse(localStorage.getItem('reyna_user')); } catch { return null; }
}

export function isLoggedIn() {
  return !!localStorage.getItem('reyna_token');
}

export function logout() {
  localStorage.removeItem('reyna_token');
  localStorage.removeItem('reyna_user');
}

export function getToken() {
  return localStorage.getItem('reyna_token');
}
