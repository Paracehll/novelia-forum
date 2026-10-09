import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { registerHooks, stripTypeScriptTypes } from 'node:module';
import test from 'node:test';
import { effectScope, ref } from 'vue';

// Keep Vue's real refs, synchronous watcher and effect-scope disposal; mock only
// the API, session and content-cache boundaries, as in post-actions.test.mjs.
const fixture = {
  whoami: ref({ user: undefined, isSignedIn: false }),
  calls: [],
  notices: [],
  listRequests: [],
  removeRequests: [],
  cacheCalls: [],
  posts: [],
  comments: [],
};
globalThis.__blacklistManagementTest = fixture;

const composableUrl = new URL(
  '../src/composables/useBlacklistManagement.ts',
  import.meta.url,
).href;
const mocks = {
  '@/api': `
    const f = globalThis.__blacklistManagementTest;
    export const getBlacklist = signal => {
      f.calls.push(['getBlacklist', signal]);
      return f.listRequests.shift().promise;
    };
    export const setUserBlocked = (...args) => {
      f.calls.push(['setUserBlocked', ...args]);
      return f.removeRequests.shift().promise;
    };
  `,
  '@novelia/web-kit': `
    const f = globalThis.__blacklistManagementTest;
    export const useWebKit = () => ({ whoami: f.whoami });
    export const getApiErrorMessage = async (_, fallback) => fallback;
    export const Notify = {
      success: message => f.notices.push(['success', message]),
      error: message => f.notices.push(['error', message]),
    };
  `,
  ...Object.fromEntries(
    ['post', 'comment'].map((name) => [
      `@/stores/${name}`,
      `
      const f = globalThis.__blacklistManagementTest;
      export const use${name === 'post' ? 'Post' : 'Comment'}Store = () => ({
        refreshBlacklist(userId, blocked) {
          f.cacheCalls.push(['${name}', userId, blocked]);
          for (const item of f.${name}s) {
            if (item.authorId === userId) item.authorBlocked = blocked;
          }
        },
      });
    `,
    ]),
  ),
};

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier in mocks)
      return {
        url: `blacklist-management-test:${specifier}`,
        shortCircuit: true,
      };
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    const prefix = 'blacklist-management-test:';
    if (url.startsWith(prefix)) {
      return {
        format: 'module',
        source: mocks[url.slice(prefix.length)],
        shortCircuit: true,
      };
    }
    if (url === composableUrl) {
      return {
        format: 'module',
        source: stripTypeScriptTypes(readFileSync(new URL(url), 'utf8')),
        shortCircuit: true,
      };
    }
    return nextLoad(url, context);
  },
});

const { useBlacklistManagement } = await import(composableUrl);

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function signIn(id) {
  fixture.whoami.value = {
    user: id === undefined ? undefined : { id },
    isSignedIn: id !== undefined,
  };
}

function setup(viewerId = 7) {
  signIn(viewerId ?? undefined);
  for (const key of [
    'calls',
    'notices',
    'listRequests',
    'removeRequests',
    'cacheCalls',
  ])
    fixture[key].length = 0;
  fixture.posts = [
    { authorId: 10, authorBlocked: true },
    { authorId: 11, authorBlocked: true },
  ];
  fixture.comments = [
    { authorId: 10, authorBlocked: true },
    { authorId: 11, authorBlocked: true },
  ];
  const initial = deferred();
  if (viewerId != null) fixture.listRequests.push(initial);
  const scope = effectScope();
  const management = scope.run(() => useBlacklistManagement());
  return { scope, management, initial };
}

const entries = [
  { userId: 10, username: 'blocked-one' },
  { userId: 11, username: 'blocked-two' },
];

async function flush() {
  // Covers load's catch -> asynchronous error-message formatter -> finally.
  for (let i = 0; i < 5; i++) await Promise.resolve();
}

async function loaded(context) {
  context.initial.resolve({ items: entries });
  await flush();
  return context;
}

function removalCalls() {
  return fixture.calls.filter(([name]) => name === 'setUserBlocked');
}

test('guest does not load or submit removal; signing in starts loading', async () => {
  const { scope, management } = setup(null);
  try {
    await management.load();
    management.selected.value = entries[0];
    await management.confirmRemove();
    assert.deepEqual(fixture.calls, []);
    assert.equal(management.loading.value, false);
    assert.deepEqual(management.entries.value, []);
    const request = deferred();
    fixture.listRequests.push(request);
    signIn(7);
    assert.equal(fixture.calls.length, 1);
    assert.equal(management.loading.value, true);
    assert.equal(management.selected.value, undefined);
    request.resolve({ items: entries });
    await flush();
    assert.deepEqual(management.entries.value, entries);
  } finally {
    scope.stop();
  }
});

test('signed-in scope loads immediately with an abort signal', async () => {
  const context = setup();
  const { scope, management } = context;
  try {
    assert.equal(management.loading.value, true);
    assert.equal(fixture.calls.length, 1);
    assert.equal(fixture.calls[0][0], 'getBlacklist');
    assert.ok(fixture.calls[0][1] instanceof AbortSignal);
    assert.equal(fixture.calls[0][1].aborted, false);
    await loaded(context);
    assert.deepEqual(management.entries.value, entries);
    assert.equal(management.loading.value, false);
    assert.equal(management.error.value, '');
  } finally {
    scope.stop();
  }
});

test('load failure exposes an error and retry clears it and replaces entries', async () => {
  const { scope, management, initial } = setup();
  try {
    initial.reject(new Error('network'));
    await flush();
    assert.equal(management.error.value, '加载黑名单失败');
    assert.equal(management.loading.value, false);
    assert.deepEqual(management.entries.value, []);
    const retry = deferred();
    fixture.listRequests.push(retry);
    const pending = management.load();
    assert.equal(management.error.value, '');
    assert.equal(management.loading.value, true);
    assert.equal(fixture.calls[0][1].aborted, true);
    retry.resolve({ items: entries });
    await pending;
    assert.deepEqual(management.entries.value, entries);
    assert.equal(management.loading.value, false);
    assert.deepEqual(fixture.notices, []);
  } finally {
    scope.stop();
  }
});

test('successful removal updates both caches, removes only the selected entry and closes confirmation', async () => {
  const { scope, management } = await loaded(setup());
  try {
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.removeRequests.push(request);
    const pending = management.confirmRemove();
    assert.equal(management.removing.value, true);
    assert.deepEqual(management.entries.value, entries);
    request.resolve();
    await pending;
    assert.deepEqual(removalCalls(), [['setUserBlocked', 10, false]]);
    assert.deepEqual(fixture.cacheCalls, [
      ['post', 10, false],
      ['comment', 10, false],
    ]);
    for (const cache of [fixture.posts, fixture.comments]) {
      assert.equal(cache[0].authorBlocked, false);
      assert.equal(cache[1].authorBlocked, true);
    }
    assert.deepEqual(management.entries.value, [entries[1]]);
    assert.equal(management.selected.value, undefined);
    assert.equal(management.removing.value, false);
    assert.deepEqual(fixture.notices, [['success', '已取消拉黑']]);
  } finally {
    scope.stop();
  }
});

test('removal failure keeps entries and selection, leaves caches unchanged and allows retry', async () => {
  const { scope, management } = await loaded(setup());
  try {
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.removeRequests.push(request);
    const pending = management.confirmRemove();
    request.reject(new Error('network'));
    await pending;
    assert.deepEqual(management.entries.value, entries);
    assert.deepEqual(management.selected.value, entries[0]);
    assert.equal(management.removing.value, false);
    assert.deepEqual(fixture.cacheCalls, []);
    assert.ok(
      [...fixture.posts, ...fixture.comments].every(
        (item) => item.authorBlocked,
      ),
    );
    assert.deepEqual(fixture.notices, [['error', '取消拉黑失败']]);
    const retry = deferred();
    fixture.removeRequests.push(retry);
    const retried = management.confirmRemove();
    retry.resolve();
    await retried;
    assert.equal(removalCalls().length, 2);
    assert.deepEqual(management.entries.value, [entries[1]]);
  } finally {
    scope.stop();
  }
});

test('duplicate confirmations share a single in-flight removal lock', async () => {
  const { scope, management } = await loaded(setup());
  try {
    await management.confirmRemove();
    assert.deepEqual(removalCalls(), []);
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.removeRequests.push(request);
    const pending = management.confirmRemove();
    await management.confirmRemove();
    await management.confirmRemove();
    assert.equal(removalCalls().length, 1);
    assert.equal(management.removing.value, true);
    request.resolve();
    await pending;
    assert.equal(management.removing.value, false);
    assert.equal(fixture.notices.length, 1);
  } finally {
    scope.stop();
  }
});

test('removal invalidates an older pending list so it cannot restore the entry', async () => {
  const { scope, management } = await loaded(setup());
  try {
    const list = deferred();
    fixture.listRequests.push(list);
    const loading = management.load();
    const signal = fixture.calls.at(-1)[1];
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.removeRequests.push(request);
    const pending = management.confirmRemove();
    request.resolve();
    await pending;
    assert.equal(signal.aborted, true);
    assert.equal(management.loading.value, false);
    list.resolve({ items: entries });
    await loading;
    assert.deepEqual(management.entries.value, [entries[1]]);
  } finally {
    scope.stop();
  }
});

test('account switch ignores late list success and failure without clearing the new loading state', async () => {
  for (const fails of [false, true]) {
    const { scope, management, initial } = setup();
    try {
      management.selected.value = entries[0];
      const current = deferred();
      fixture.listRequests.push(current);
      signIn(8);
      assert.equal(fixture.calls[0][1].aborted, true);
      assert.deepEqual(management.entries.value, []);
      assert.equal(management.selected.value, undefined);
      if (fails) initial.reject(new Error('old network error'));
      else initial.resolve({ items: entries });
      await flush();
      assert.deepEqual(management.entries.value, []);
      assert.equal(management.error.value, '');
      assert.equal(management.loading.value, true);
      current.resolve({ items: [entries[1]] });
      await flush();
      assert.deepEqual(management.entries.value, [entries[1]]);
      assert.equal(management.loading.value, false);
    } finally {
      scope.stop();
    }
  }
});

test('switching away and back ignores stale list and removal responses and preserves the current lock', async () => {
  for (const fails of [false, true]) {
    const { scope, management } = await loaded(setup());
    try {
      management.selected.value = management.entries.value[0];
      const oldRemove = deferred();
      fixture.removeRequests.push(oldRemove);
      const oldPending = management.confirmRemove();
      const awayList = deferred(),
        returnList = deferred();
      fixture.listRequests.push(awayList, returnList);
      signIn(8);
      assert.equal(management.removing.value, false);
      signIn(7);
      awayList.resolve({ items: [entries[1]] });
      await flush();
      assert.deepEqual(management.entries.value, []);
      assert.equal(management.loading.value, true);
      returnList.resolve({ items: entries });
      await flush();
      management.selected.value = management.entries.value[1];
      const currentRemove = deferred();
      fixture.removeRequests.push(currentRemove);
      const currentPending = management.confirmRemove();
      if (fails) oldRemove.reject(new Error('old removal error'));
      else oldRemove.resolve();
      await oldPending;
      assert.deepEqual(management.entries.value, entries);
      assert.deepEqual(management.selected.value, entries[1]);
      assert.equal(management.removing.value, true);
      assert.deepEqual(fixture.cacheCalls, []);
      assert.deepEqual(fixture.notices, []);
      currentRemove.resolve();
      await currentPending;
      assert.deepEqual(management.entries.value, [entries[0]]);
      assert.deepEqual(fixture.cacheCalls, [
        ['post', 11, false],
        ['comment', 11, false],
      ]);
      assert.equal(management.removing.value, false);
    } finally {
      scope.stop();
    }
  }
});

test('signing out clears entries and selection and ignores late list results', async () => {
  const { scope, management } = await loaded(setup());
  try {
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.listRequests.push(request);
    const pending = management.load();
    signIn(undefined);
    assert.deepEqual(management.entries.value, []);
    assert.equal(management.selected.value, undefined);
    assert.equal(management.loading.value, false);
    assert.equal(fixture.calls.at(-1)[1].aborted, true);
    request.resolve({ items: entries });
    await pending;
    assert.deepEqual(management.entries.value, []);
  } finally {
    scope.stop();
  }
});

test('scope disposal aborts loading and suppresses late list data and errors', async () => {
  for (const fails of [false, true]) {
    const { scope, management, initial } = setup();
    scope.stop();
    assert.equal(fixture.calls[0][1].aborted, true);
    if (fails) initial.reject(new Error('late load error'));
    else initial.resolve({ items: entries });
    await flush();
    assert.deepEqual(management.entries.value, []);
    assert.equal(management.error.value, '');
    assert.deepEqual(fixture.notices, []);
    const count = fixture.calls.length;
    await management.load();
    assert.equal(fixture.calls.length, count);
  }
});

test('scope disposal suppresses removal notices; successful server updates still refresh both content caches', async () => {
  for (const fails of [false, true]) {
    const { scope, management } = await loaded(setup());
    management.selected.value = management.entries.value[0];
    const request = deferred();
    fixture.removeRequests.push(request);
    const pending = management.confirmRemove();
    scope.stop();
    if (fails) request.reject(new Error('late removal error'));
    else request.resolve();
    await pending;
    assert.deepEqual(fixture.notices, []);
    assert.deepEqual(management.entries.value, entries);
    assert.deepEqual(
      fixture.cacheCalls,
      fails
        ? []
        : [
            ['post', 10, false],
            ['comment', 10, false],
          ],
    );
    await management.confirmRemove();
    assert.equal(removalCalls().length, 1);
  }
});
