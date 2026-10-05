import { createRouter, createWebHistory } from 'vue-router';

import { useCategoryStore } from '@/stores/category';
import { setPostListReturn } from '@/utils/postNavigation';
import CommunityRulesView from '@/views/community-rules/CommunityRulesView.vue';
import FavoritePostListView from '@/views/post-list/FavoritePostListView.vue';
import MyPostListView from '@/views/post-list/MyPostListView.vue';
import PostDetailView from '@/views/post-detail/PostDetailView.vue';
import PostCreateView from '@/views/post-editor/PostCreateView.vue';
import PostEditView from '@/views/post-editor/PostEditView.vue';
import PostListView from '@/views/post-list/PostListView.vue';
import { MyStrikeListView } from '@novelia/web-kit';

function defaultCategorySlug() {
  return useCategoryStore().defaultCategory.slug;
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      redirect: {
        name: 'posts',
        params: { slug: 'announcements' },
      },
    },
    {
      path: '/c/:slug',
      name: 'posts',
      component: PostListView,
      meta: { title: '讨论' },
      beforeEnter: (to) => {
        const categoryStore = useCategoryStore();
        return categoryStore.categories.some(
          (category) => category.slug === to.params.slug,
        )
          ? true
          : {
              name: 'posts',
              params: { slug: categoryStore.defaultCategory.slug },
            };
      },
    },
    {
      path: '/rules',
      name: 'community-rules',
      component: CommunityRulesView,
      meta: { title: '社区守则' },
    },
    {
      path: '/favorites',
      name: 'favorites',
      component: FavoritePostListView,
      meta: { title: '我的收藏' },
    },
    {
      path: '/my/posts',
      name: 'my-posts',
      component: MyPostListView,
      meta: { title: '我的帖子' },
    },
    {
      path: '/strikes',
      name: 'strikes',
      component: MyStrikeListView,
      meta: { title: '处罚记录' },
    },
    {
      path: '/posts/new',
      name: 'post-create',
      component: PostCreateView,
      meta: { title: '发表帖子' },
    },
    {
      path: '/p/:id/edit',
      name: 'post-edit',
      component: PostEditView,
      meta: { title: '编辑帖子' },
    },
    {
      path: '/p/:id',
      name: 'post-detail',
      component: PostDetailView,
      meta: { title: '帖子详情' },
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: () => ({
        name: 'posts',
        params: { slug: defaultCategorySlug() },
      }),
    },
  ],
});

router.afterEach((to, from, failure) => {
  if (failure) return;
  if (
    to.name === 'post-detail' &&
    ['posts', 'favorites', 'my-posts'].includes(String(from.name))
  ) {
    setPostListReturn({
      path: from.fullPath,
      scrollTop: document.querySelector('main')?.scrollTop ?? 0,
    });
  } else if (!(
    ['post-detail', 'post-edit'].includes(String(to.name)) &&
    ['post-detail', 'post-edit'].includes(String(from.name)) &&
    to.params.id === from.params.id
  )) {
    setPostListReturn();
  }
  document.title = `${String(to.meta.title ?? '社区')} | Novelia Forum`;
});

export default router;
