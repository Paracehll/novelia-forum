import { createRouter, createWebHistory } from 'vue-router';

import CategoriesView from '@/views/categories/CategoriesView.vue';
import CommentsView from '@/views/comments/CommentsView.vue';
import OverviewView from '@/views/overview/OverviewView.vue';
import PostsView from '@/views/posts/PostsView.vue';

const APP_TITLE = '论坛服务管理后台';

const router = createRouter({
  history: createWebHistory('/admin/'),
  routes: [
    {
      path: '/login',
      name: 'login',
      redirect: { name: 'overview' },
    },
    {
      path: '/',
      redirect: { name: 'overview' },
      children: [
        {
          path: 'overview',
          name: 'overview',
          component: OverviewView,
          meta: { title: '概览' },
        },
        {
          path: 'categories',
          name: 'categories',
          component: CategoriesView,
          meta: { title: '标签管理' },
        },
        {
          path: 'posts',
          name: 'posts',
          component: PostsView,
          meta: { title: '帖子管理' },
        },
        {
          path: 'comments',
          name: 'comments',
          component: CommentsView,
          meta: { title: '评论管理' },
        },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: { name: 'overview' } },
  ],
});

router.afterEach((to) => {
  const pageTitle = to.meta.title;
  document.title = pageTitle
    ? `${String(pageTitle)} | ${APP_TITLE}`
    : APP_TITLE;
});

export default router;
