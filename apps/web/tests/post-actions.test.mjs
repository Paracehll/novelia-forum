import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { registerHooks, stripTypeScriptTypes } from 'node:module';
import test from 'node:test';
import { compileScript, parse } from 'vue/compiler-sfc';
import { computed, effectScope, reactive, ref } from 'vue';

// Exercise the real composable and SFC setup with mocked API/cache boundaries.
// Node 22.15+ provides registerHooks; no test framework dependency is needed.
const fixture = {
  auth: ref(),
  post: ref(),
  calls: [],
  notices: [],
  requests: [],
};
globalThis.__postActionsTest = fixture;
globalThis.__postActionsVue = { computed };

const favoriteUrl = new URL(
  '../src/composables/usePostFavorite.ts',
  import.meta.url,
).href;
const actionsUrl = new URL(
  '../src/views/post-detail/PostActions.vue',
  import.meta.url,
).href;
const mocks = {
  '@/api': `
    const f = globalThis.__postActionsTest;
    ${['setPostFavorite', 'deletePost', 'lockPost', 'unlockPost', 'pinPost', 'unpinPost', 'setPostStatus'].map((name) => `export const ${name} = (...args) => { f.calls.push(['${name}', ...args]); return f.requests.shift().promise; };`).join('\n')}
  `,
  '@/stores/post': `
    export const usePostStore = () => ({ setPost: post => { globalThis.__postActionsTest.post.value = post; } });
  `,
  // Mirror web-kit: components read the session view from the kit, not a token profile.
  '@novelia/web-kit': `
    const f = globalThis.__postActionsTest;
    const { computed } = globalThis.__postActionsVue;
    const isAdmin = user => user?.role === 'admin';
    const whoami = computed(() => {
      const user = f.auth.value;
      return {
        user,
        isSignedIn: user !== undefined,
        isAdmin: isAdmin(user),
        asAdmin: isAdmin(user) && user?.adminMode === true,
      };
    });
    export const useWebKit = () => ({ whoami });
    export const Notify = {
      success: message => f.notices.push(['success', message]),
      error: message => f.notices.push(['error', message]),
    };
    export const getApiErrorMessage = async (_, fallback) => fallback;
    export const XButton = {}, XActionMenuItem = {}, XConfirmDialog = {};
  `,
  '@vicons/material':
    'export const ChatBubbleOutlineOutlined = {}, EditOutlined = {}, MoreVertOutlined = {}, ShareOutlined = {};',
  'reka-ui':
    'export const DropdownMenuContent = {}, DropdownMenuPortal = {}, DropdownMenuRoot = {}, DropdownMenuTrigger = {};',
  './PostFavoriteButton.vue': 'export default {};',
  './PostShareDialog.vue': 'export default {};',
  '@/components/UserBlacklistDialog.vue': 'export default {};',
  '@/components/UserModerationDialog.vue': 'export default {};',
};

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier in mocks)
      return { url: `post-test:${specifier}`, shortCircuit: true };
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    if (url.startsWith('post-test:')) {
      return {
        format: 'module',
        source: mocks[url.slice('post-test:'.length)],
        shortCircuit: true,
      };
    }
    if (url === favoriteUrl) {
      return {
        format: 'module',
        source: stripTypeScriptTypes(readFileSync(new URL(url), 'utf8')),
        shortCircuit: true,
      };
    }
    if (url === actionsUrl) {
      const { descriptor } = parse(readFileSync(new URL(url), 'utf8'));
      const script = compileScript(descriptor, { id: 'post-actions-test' });
      return {
        format: 'module',
        source: stripTypeScriptTypes(script.content),
        shortCircuit: true,
      };
    }
    return nextLoad(url, context);
  },
});

const { usePostFavorite } =
  await import('../src/composables/usePostFavorite.ts');
const { default: PostActions } = await import(actionsUrl);

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function setup(role = 'admin') {
  fixture.auth.value = role
    ? { id: 7, role, adminMode: role === 'admin' }
    : undefined;
  fixture.post.value = {
    id: 10,
    authorId: 7,
    title: 'Post',
    createdAt: new Date().toISOString(),
    favorited: false,
    commentsLocked: false,
    pinOrder: null,
    commentsCount: 3,
    status: 0,
  };
  fixture.calls.length = 0;
  fixture.notices.length = 0;
  fixture.requests.length = 0;
  const scope = effectScope();
  const favorite = scope.run(() => usePostFavorite(fixture.post));
  const props = reactive({
    post: fixture.post.value,
    canFavorite: !!role,
    favoriteLoading: false,
  });
  const events = [];
  const actions = scope.run(() =>
    PostActions.setup(props, {
      expose() {},
      emit(name, patch) {
        events.push([name, patch]);
        if (name === 'updated')
          fixture.post.value = { ...fixture.post.value, ...patch };
      },
    }),
  );
  return { scope, favorite, actions, props, events };
}

async function settle(request) {
  request.resolve();
  await request.promise;
  await Promise.resolve();
}

test('copy original is available to guests and preserves raw Markdown', async (t) => {
  const { scope, actions, props } = setup(null);
  const content = '# 标题\n\n**原文** [链接](https://example.com)\n';
  props.post.content = content;
  const writeText = t.mock.fn(async () => {});
  const descriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });
  try {
    const { descriptor: sfc } = parse(
      readFileSync(new URL(actionsUrl), 'utf8'),
    );
    assert.match(sfc.template.content, /<DropdownMenuRoot>/);
    assert.match(sfc.template.content, /@activate="copyPost"/);
    await actions.copyPost();
    assert.equal(writeText.mock.calls[0].arguments[0], content);
    assert.deepEqual(fixture.notices, [['success', '帖子原文已复制']]);
    assert.equal(actions.copying.value, false);
  } finally {
    scope.stop();
    if (descriptor) Object.defineProperty(navigator, 'clipboard', descriptor);
    else delete navigator.clipboard;
  }
});

test('copy original prevents duplicate writes and unlocks after clipboard failure', async (t) => {
  const { scope, actions } = setup();
  const request = deferred();
  const writeText = t.mock.fn(() => request.promise);
  const descriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });
  try {
    const pending = actions.copyPost();
    await actions.copyPost();
    assert.equal(writeText.mock.calls.length, 1);
    assert.equal(actions.copying.value, true);
    request.reject(new Error('clipboard denied'));
    await pending;
    assert.equal(actions.copying.value, false);
    assert.deepEqual(fixture.notices, [
      ['error', '复制失败，请手动选择帖子内容复制'],
    ]);
  } finally {
    scope.stop();
    if (descriptor) Object.defineProperty(navigator, 'clipboard', descriptor);
    else delete navigator.clipboard;
  }
});

test('two favorite entrances share a single request lock and source of truth', async () => {
  const { scope, favorite } = setup();
  try {
    const request = deferred();
    fixture.requests.push(request);
    const first = favorite.toggle();
    await favorite.toggle();
    assert.equal(favorite.loading.value, true);
    assert.deepEqual(fixture.calls, [['setPostFavorite', 10, true]]);
    await settle(request);
    await first;
    assert.equal(fixture.post.value.favorited, true);
    assert.equal(favorite.loading.value, false);
  } finally {
    scope.stop();
  }
});

test('guest cannot submit a favorite request', async () => {
  const { scope, favorite } = setup(null);
  try {
    await favorite.toggle();
    assert.equal(favorite.canFavorite.value, false);
    assert.equal(fixture.calls.length, 0);
  } finally {
    scope.stop();
  }
});

test('favorite failure preserves data and unlocks both entrances', async () => {
  const { scope, favorite } = setup();
  try {
    const request = deferred();
    fixture.requests.push(request);
    const pending = favorite.toggle();
    request.reject(new Error('network'));
    await pending;
    assert.equal(fixture.post.value.favorited, false);
    assert.equal(favorite.loading.value, false);
    assert.deepEqual(fixture.notices, [['error', '更新收藏失败']]);
  } finally {
    scope.stop();
  }
});

test('leaving and returning to same post ignores old response and preserves new lock', async () => {
  const { scope, favorite } = setup();
  try {
    const old = deferred(),
      current = deferred();
    fixture.requests.push(old, current);
    const first = favorite.toggle();
    const original = fixture.post.value;
    fixture.post.value = { ...original, id: 11 };
    fixture.post.value = original;
    const second = favorite.toggle();
    await settle(old);
    await first;
    assert.equal(fixture.post.value.favorited, false);
    assert.equal(favorite.loading.value, true);
    assert.equal(fixture.notices.length, 0);
    await settle(current);
    await second;
    assert.equal(fixture.post.value.favorited, true);
  } finally {
    scope.stop();
  }
});

test('viewer change and scope disposal suppress late favorite responses', async () => {
  for (const change of ['viewer', 'dispose']) {
    const { scope, favorite } = setup();
    const request = deferred();
    fixture.requests.push(request);
    const pending = favorite.toggle();
    if (change === 'viewer')
      fixture.auth.value = { id: 8, role: 'admin', adminMode: true };
    else scope.stop();
    await settle(request);
    await pending;
    assert.equal(fixture.post.value.favorited, false);
    assert.equal(fixture.notices.length, 0);
    scope.stop();
  }
});

test('management starts API only after acquiring shared management lock', async () => {
  const { scope, actions, events } = setup();
  try {
    const request = deferred();
    fixture.requests.push(request);
    actions.togglePin();
    actions.toggleLock();
    assert.deepEqual(fixture.calls, [['pinPost', 10, 0]]);
    await settle(request);
    assert.deepEqual(events, [['updated', { pinOrder: 0 }]]);
    assert.equal(actions.actionLoading.value, false);
  } finally {
    scope.stop();
  }
});

test('management and favorite preserve each other and new comment data in either completion order', async () => {
  for (const favoriteFirst of [true, false]) {
    const { scope, favorite, actions } = setup();
    try {
      const favoriteRequest = deferred(),
        manageRequest = deferred();
      fixture.requests.push(favoriteRequest, manageRequest);
      const pending = favorite.toggle();
      actions.toggleLock();
      fixture.post.value = { ...fixture.post.value, commentsCount: 4 };
      await settle(favoriteFirst ? favoriteRequest : manageRequest);
      await settle(favoriteFirst ? manageRequest : favoriteRequest);
      await pending;
      assert.equal(fixture.post.value.favorited, true);
      assert.equal(fixture.post.value.commentsLocked, true);
      assert.equal(fixture.post.value.commentsCount, 4);
    } finally {
      scope.stop();
    }
  }
});

test('management disposal or viewer change suppresses late events and notifications', async () => {
  for (const change of ['viewer', 'dispose']) {
    const { scope, actions, events } = setup();
    const request = deferred();
    fixture.requests.push(request);
    actions.togglePin();
    if (change === 'viewer')
      fixture.auth.value = { id: 8, role: 'admin', adminMode: true };
    else scope.stop();
    await settle(request);
    assert.equal(events.length, 0);
    assert.equal(fixture.notices.length, 0);
    scope.stop();
  }
});

test('management failure does not patch data and releases lock', async () => {
  const { scope, actions, events } = setup();
  try {
    const request = deferred();
    fixture.requests.push(request);
    const pending = actions.updateModeration(
      () => request.promise,
      { commentsLocked: true },
      'ok',
      'failed',
    );
    request.reject(new Error('network'));
    await pending;
    assert.equal(events.length, 0);
    assert.equal(actions.actionLoading.value, false);
    assert.deepEqual(fixture.notices, [['error', 'failed']]);
  } finally {
    scope.stop();
  }
});

test('toggling admin mode does not reset the favorite lock', async () => {
  // Favoriting has no admin-mode variant, so the mode is not part of the
  // viewer identity that invalidates in-flight work.
  const { scope, favorite } = setup();
  try {
    const request = deferred();
    fixture.requests.push(request);
    const pending = favorite.toggle();
    assert.equal(favorite.loading.value, true);

    fixture.auth.value = { id: 7, role: 'admin', adminMode: false };
    assert.equal(favorite.loading.value, true);

    await settle(request);
    await pending;
    assert.equal(fixture.post.value.favorited, true);
    assert.deepEqual(fixture.calls, [['setPostFavorite', 10, true]]);
  } finally {
    scope.stop();
  }
});

test('permissions retain owner/admin editing and deletion rules', () => {
  const { scope, actions, props } = setup('member');
  try {
    assert.equal(actions.canManagePost.value, true);
    assert.equal(actions.canDelete.value, true);
    props.post = { ...props.post, createdAt: '2000-01-01T00:00:00Z' };
    assert.equal(actions.canManagePost.value, true);
    assert.equal(actions.canDelete.value, false);
    fixture.auth.value = { id: 8, role: 'member', adminMode: false };
    assert.equal(actions.canManagePost.value, false);
    fixture.auth.value = { id: 8, role: 'admin', adminMode: true };
    assert.equal(actions.canManagePost.value, true);
    assert.equal(actions.canDelete.value, true);
    assert.equal(actions.canModerateAuthor.value, true);
  } finally {
    scope.stop();
  }
});

test('admin UI follows the admin-mode toggle, not the role alone', () => {
  // An admin browsing in normal mode must see the same page as a regular user;
  // moderation affordances appear only after the account menu enables admin mode.
  const { scope, actions } = setup('member');
  try {
    fixture.auth.value = { id: 8, role: 'admin', adminMode: false };
    assert.equal(actions.canManagePost.value, false);
    assert.equal(actions.canModerateAuthor.value, false);
    assert.equal(actions.canDelete.value, false);

    fixture.auth.value = { id: 8, role: 'admin', adminMode: true };
    assert.equal(actions.canManagePost.value, true);
    assert.equal(actions.canModerateAuthor.value, true);
    assert.equal(actions.canDelete.value, true);
  } finally {
    scope.stop();
  }
});
