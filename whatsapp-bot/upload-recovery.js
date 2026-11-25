const fs = require('fs/promises');
const path = require('path');
const { createHash, randomUUID } = require('crypto');

const MAX_UPLOAD_BYTES = 49 * 1024 * 1024;
const MAX_PENDING_UPLOADS = 50;
const MAX_PENDING_BYTES = 256 * 1024 * 1024;
const MAX_AUTOMATIC_RETRIES = 3;

const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

function isRetryableStatus(status) {
  return status === 408 || status === 425 || status === 429 || (status >= 500 && status <= 599);
}

function isAcceptedUpload(response) {
  return response?.status === 'staged' || response?.status === 'duplicate';
}

function retryDelay(retryIndex) {
  return Math.min(30_000, 500 * (2 ** retryIndex));
}

async function withRetries(attempt, {
  maxRetries = MAX_AUTOMATIC_RETRIES,
  sleepFn = sleep,
  delayFn = retryDelay,
} = {}) {
  let retries = 0;
  while (true) {
    let result;
    try {
      result = await attempt();
    } catch (error) {
      result = { ok: false, retryable: true, error: error.message || String(error) };
    }

    if (result.ok || !result.retryable || retries >= maxRetries) {
      return { ...result, retries };
    }

    await sleepFn(delayFn(retries));
    retries++;
  }
}

async function atomicWrite(filePath, data) {
  const tempPath = `${filePath}.${randomUUID()}.tmp`;
  let handle;
  try {
    handle = await fs.open(tempPath, 'wx', 0o600);
    await handle.writeFile(data);
    await handle.sync();
    await handle.close();
    handle = null;
    await fs.rename(tempPath, filePath);
  } catch (error) {
    if (handle) await handle.close().catch(() => {});
    await fs.unlink(tempPath).catch(() => {});
    throw error;
  }
}

class PendingUploadQueue {
  constructor({
    directory = path.join(__dirname, 'pending_uploads'),
    maxItems = MAX_PENDING_UPLOADS,
    maxBytes = MAX_PENDING_BYTES,
    maxFileBytes = MAX_UPLOAD_BYTES,
  } = {}) {
    this.directory = directory;
    this.maxItems = maxItems;
    this.maxBytes = maxBytes;
    this.maxFileBytes = maxFileBytes;
    this.initialized = false;
    this.operation = Promise.resolve();
  }

  async initialize() {
    await fs.mkdir(this.directory, { recursive: true, mode: 0o700 });
    await fs.chmod(this.directory, 0o700);
    this.initialized = true;

    const names = await fs.readdir(this.directory);
    for (const name of names.filter(entry => entry.endsWith('.tmp'))) {
      console.warn(`[UPLOAD-QUEUE] Removing incomplete temporary entry ${name}`);
      await fs.unlink(path.join(this.directory, name));
    }
    const remaining = await fs.readdir(this.directory);
    const metadataIds = new Set(remaining.filter(name => name.endsWith('.json')).map(name => name.slice(0, -5)));
    for (const name of remaining.filter(entry => entry.endsWith('.bin'))) {
      if (!metadataIds.has(name.slice(0, -4))) {
        console.warn(`[UPLOAD-QUEUE] Removing incomplete orphaned data file ${name}`);
        await fs.unlink(path.join(this.directory, name));
      }
    }

    return this.list();
  }

  #exclusive(operation) {
    const run = this.operation.then(operation, operation);
    this.operation = run.catch(() => {});
    return run;
  }

  #assertInitialized() {
    if (!this.initialized) throw new Error('Pending upload queue is not initialized');
  }

  #metadataPath(id) {
    if (!/^[\da-f-]{36}$/i.test(id)) throw new Error('Invalid pending upload ID');
    return path.join(this.directory, `${id}.json`);
  }

  #dataPath(id) {
    if (!/^[\da-f-]{36}$/i.test(id)) throw new Error('Invalid pending upload ID');
    return path.join(this.directory, `${id}.bin`);
  }

  async #readEntries() {
    const names = await fs.readdir(this.directory);
    const entries = [];
    for (const name of names.filter(entry => entry.endsWith('.json'))) {
      const metadataPath = path.join(this.directory, name);
      const metadata = JSON.parse(await fs.readFile(metadataPath, 'utf8'));
      if (!metadata.id || name !== `${metadata.id}.json`) {
        throw new Error(`Invalid pending upload metadata: ${name}`);
      }
      const stat = await fs.stat(this.#dataPath(metadata.id));
      entries.push({ ...metadata, storedBytes: stat.size });
    }
    return entries.sort((a, b) => a.createdAt - b.createdAt);
  }

  async list(groupJid) {
    this.#assertInitialized();
    const entries = await this.#readEntries();
    return groupJid ? entries.filter(entry => entry.groupJid === groupJid) : entries;
  }

  async enqueue(upload, fileBuffer) {
    this.#assertInitialized();
    return this.#exclusive(async () => {
      const bytes = Buffer.from(fileBuffer);
      if (bytes.length > this.maxFileBytes) {
        throw new Error(`File exceeds the ${Math.floor(this.maxFileBytes / (1024 * 1024))} MiB upload limit`);
      }

      const entries = await this.#readEntries();
      const duplicate = upload.sourceMessageId && entries.find(entry =>
        entry.groupJid === upload.groupJid && entry.sourceMessageId === upload.sourceMessageId
      );
      if (duplicate) return duplicate;

      const storedBytes = entries.reduce((sum, entry) => sum + entry.storedBytes, 0);
      if (entries.length >= this.maxItems || storedBytes + bytes.length > this.maxBytes) {
        throw new Error('Pending upload queue is full; no file was discarded');
      }

      const id = randomUUID();
      const now = Date.now();
      const metadata = {
        ...upload,
        id,
        createdAt: now,
        updatedAt: now,
        attemptCount: 0,
        retryable: true,
        lastError: '',
        bufferLength: bytes.length,
        contentHash: createHash('sha256').update(bytes).digest('hex'),
      };
      delete metadata.storedBytes;

      const dataPath = this.#dataPath(id);
      const metadataPath = this.#metadataPath(id);
      await atomicWrite(dataPath, bytes);
      try {
        await atomicWrite(metadataPath, JSON.stringify(metadata));
      } catch (error) {
        await fs.unlink(dataPath).catch(() => {});
        throw error;
      }
      return { ...metadata, storedBytes: bytes.length };
    });
  }

  async readBuffer(id) {
    this.#assertInitialized();
    const [metadata, bytes] = await Promise.all([
      fs.readFile(this.#metadataPath(id), 'utf8').then(JSON.parse),
      fs.readFile(this.#dataPath(id)),
    ]);
    const contentHash = createHash('sha256').update(bytes).digest('hex');
    if (bytes.length !== metadata.bufferLength || contentHash !== metadata.contentHash) {
      throw new Error(`Pending upload ${id} failed its integrity check`);
    }
    return bytes;
  }

  async update(id, patch) {
    this.#assertInitialized();
    return this.#exclusive(async () => {
      const metadataPath = this.#metadataPath(id);
      const current = JSON.parse(await fs.readFile(metadataPath, 'utf8'));
      const updated = { ...current, ...patch, id, updatedAt: Date.now() };
      await atomicWrite(metadataPath, JSON.stringify(updated));
      const stat = await fs.stat(this.#dataPath(id));
      return { ...updated, storedBytes: stat.size };
    });
  }

  async remove(id) {
    this.#assertInitialized();
    return this.#exclusive(async () => {
      await fs.unlink(this.#metadataPath(id)).catch(error => {
        if (error.code !== 'ENOENT') throw error;
      });
      await fs.unlink(this.#dataPath(id)).catch(error => {
        if (error.code !== 'ENOENT') throw error;
      });
    });
  }
}

module.exports = {
  MAX_UPLOAD_BYTES,
  MAX_AUTOMATIC_RETRIES,
  PendingUploadQueue,
  isAcceptedUpload,
  isRetryableStatus,
  retryDelay,
  withRetries,
};
