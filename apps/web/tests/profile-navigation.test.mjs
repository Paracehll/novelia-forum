import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { registerHooks, stripTypeScriptTypes } from 'node:module';
import test from 'node:test';
import { createMemoryHistory, createRouter } from 'vue-router';

const routerUrl = new URL('../src/router.ts', import.meta.url).href;
const fixture = { returnTo: undefined };
globalThis.__profileNavigationTest = fixture;
globalThis.__profileRouter = { createMemoryHistory, createRouter };
globalThis.document = {
  title: '',
  querySelector: () => ({ scrollTop: 120 }),
};
const mocks = {
  'vue-router': `
    const r = globalThis.__profileRouter;
    export const createRouter = r.createRouter;
    export const createWebHistory = r.createMemoryHistory;
  `,
  '@/stores/category': `
    export const useCategoryStore = () => ({
      getLastVisitedCategory: () => 'novel',
      categories: [{ id: 1, slug: 'novel', title: '小说' }],
      saveLastVisitedCategory() {},
    });
  `,
  '@/utils/postNavigation': `
    export const setPostListReturn = value => {
      globalThis.__profileNavigationTest.returnTo = value;
    };
  `,
  '@novelia/web-kit': 'export const MyStrikeListView = {};',
};
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier in mocks || specifier.endsWith('.vue'))
      return { url: `profile-test:${specifier}`, shortCircuit: true };
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    if (url.startsWith('profile-test:'))
      return {
        format: 'module',
        source:
          mocks[url.slice('profile-test:'.length)] ?? 'export default {};',
        shortCircuit: true,
      };
    if (url === routerUrl)
      return {
        format: 'module',
        source: stripTypeScriptTypes(readFileSync(new URL(url), 'utf8')),
        shortCircuit: true,
      };
    return nextLoad(url, context);
  },
});
const { default: router } = await import(routerUrl);

test('profile entry defaults to my posts inside the profile layout', async () => {
  await router.push({ name: 'profile' });
  assert.equal(router.currentRoute.value.fullPath, '/my/posts');
  assert.deepEqual(
    router.currentRoute.value.matched.map((r) => r.name),
    ['profile', 'my-posts'],
  );
  assert.equal(document.title, '我的帖子 - 个人主页 | Novelia Forum');
});

test('legacy favorites links preserve pagination and hash', async () => {
  await router.push('/favorites?page=3#saved');
  assert.equal(
    router.currentRoute.value.fullPath,
    '/my/favorites?page=3#saved',
  );
  assert.deepEqual(
    router.currentRoute.value.matched.map((r) => r.name),
    ['profile', 'favorites'],
  );
});

test('switching profile sections resets pagination', async () => {
  await router.push('/my/posts?page=4');
  await router.push({ name: 'favorites' });
  assert.equal(router.currentRoute.value.fullPath, '/my/favorites');
});

test('both profile lists retain their return path and scroll position on detail entry', async () => {
  for (const path of ['/my/posts?page=2', '/my/favorites?page=3']) {
    await router.push(path);
    await router.push('/p/10');
    assert.deepEqual(fixture.returnTo, { path, scrollTop: 120 });
  }
});
