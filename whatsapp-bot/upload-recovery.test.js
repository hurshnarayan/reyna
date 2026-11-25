const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {
  PendingUploadQueue,
  isAcceptedUpload,
  isRetryableStatus,
  withRetries,
} = require('./upload-recovery');

test('only transient HTTP responses are retried', () => {
  for (const status of [408, 425, 429, 500, 503, 599]) {
    assert.equal(isRetryableStatus(status), true, `${status} should be retryable`);
  }
  for (const status of [400, 401, 403, 404, 413]) {
    assert.equal(isRetryableStatus(status), false, `${status} should not be retried`);
  }
});

test('only backend staged or duplicate responses count as accepted', () => {
  assert.equal(isAcceptedUpload({ status: 'staged' }), true);
  assert.equal(isAcceptedUpload({ status: 'duplicate' }), true);
  assert.equal(isAcceptedUpload({ status: 'failed' }), false);
  assert.equal(isAcceptedUpload({ reply: 'saved' }), false);
  assert.equal(isAcceptedUpload(null), false);
});

test('retries transient failures three times before succeeding', async () => {
  let calls = 0;
  const delays = [];
  const result = await withRetries(async () => {
    calls++;
    return calls < 4
      ? { ok: false, retryable: true, error: 'temporary outage' }
      : { ok: true, status: 'staged' };
  }, { sleepFn: async delay => delays.push(delay) });

  assert.equal(result.ok, true);
  assert.equal(result.retries, 3);
  assert.equal(calls, 4);
  assert.deepEqual(delays, [500, 1000, 2000]);
});

test('does not retry permanent failures', async () => {
  let calls = 0;
  const result = await withRetries(async () => {
    calls++;
    return { ok: false, retryable: false, error: 'invalid file' };
  }, { sleepFn: async () => assert.fail('permanent errors must not sleep') });

  assert.equal(calls, 1);
  assert.equal(result.retries, 0);
});

test('stops after three automatic retries when failures continue', async () => {
  let calls = 0;
  const result = await withRetries(async () => {
    calls++;
    return { ok: false, retryable: true, error: 'backend unavailable' };
  }, { sleepFn: async () => {} });

  assert.equal(calls, 4);
  assert.equal(result.retries, 3);
  assert.equal(result.ok, false);
});

test('pending uploads survive queue recreation and are removed after success', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'reyna-upload-queue-'));
  try {
    const queue = new PendingUploadQueue({ directory, maxItems: 2, maxBytes: 64 });
    await queue.initialize();
    const buffer = Buffer.from('file contents');
    const item = await queue.enqueue({
      groupJid: 'group@g.us',
      userPhone: '+15550000001',
      sourceMessageId: 'message-1',
      fileInfo: { fileName: 'notes.pdf', mimeType: 'application/pdf', fileSize: buffer.length },
    }, buffer);
    assert.equal((await fs.stat(directory)).mode & 0o777, 0o700);
    assert.equal((await fs.stat(path.join(directory, `${item.id}.bin`))).mode & 0o777, 0o600);

    const restartedQueue = new PendingUploadQueue({ directory, maxItems: 2, maxBytes: 64 });
    const entries = await restartedQueue.initialize();
    assert.equal(entries.length, 1);
    assert.equal(entries[0].id, item.id);
    assert.deepEqual(await restartedQueue.readBuffer(item.id), buffer);

    await restartedQueue.remove(item.id);
    assert.deepEqual(await restartedQueue.list(), []);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test('queue rejects modified or truncated file data', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'reyna-upload-queue-'));
  try {
    const queue = new PendingUploadQueue({ directory, maxItems: 2, maxBytes: 64 });
    await queue.initialize();
    const item = await queue.enqueue(
      { groupJid: 'group@g.us', sourceMessageId: 'message-1' },
      Buffer.from('original'),
    );
    await fs.writeFile(path.join(directory, `${item.id}.bin`), Buffer.from('tampered'));

    await assert.rejects(queue.readBuffer(item.id), /integrity check/);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test('queue does not create multiple entries for the same source message', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'reyna-upload-queue-'));
  try {
    const queue = new PendingUploadQueue({ directory, maxItems: 2, maxBytes: 64 });
    await queue.initialize();
    const upload = { groupJid: 'group@g.us', sourceMessageId: 'message-1' };
    const first = await queue.enqueue(upload, Buffer.from('first'));
    const duplicate = await queue.enqueue(upload, Buffer.from('second'));

    assert.equal(duplicate.id, first.id);
    assert.deepEqual(await queue.readBuffer(first.id), Buffer.from('first'));
    assert.equal((await queue.list()).length, 1);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test('queue capacity errors preserve existing pending uploads', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'reyna-upload-queue-'));
  try {
    const queue = new PendingUploadQueue({ directory, maxItems: 1, maxBytes: 8 });
    await queue.initialize();
    const first = await queue.enqueue({ groupJid: 'group@g.us', sourceMessageId: 'message-1' }, Buffer.from('1234'));

    await assert.rejects(
      queue.enqueue({ groupJid: 'group@g.us', sourceMessageId: 'message-2' }, Buffer.from('5678')),
      /queue is full/,
    );
    const entries = await queue.list();
    assert.equal(entries.length, 1);
    assert.equal(entries[0].id, first.id);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
