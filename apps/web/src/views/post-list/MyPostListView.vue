<script setup lang="ts">
import { ArticleOutlined } from '@vicons/material';
import { useWebKitLayout } from '@novelia/web-kit';
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { authUser } from '@/api';
import { usePostListQuery } from '@/stores/post';
import PostList from './PostList.vue';

const PAGE_SIZE = 20;

const route = useRoute();
const router = useRouter();
const { scrollToTop } = useWebKitLayout();

const page = computed(() => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
});

const totalPages = computed(() =>
  Math.max(1, Math.ceil(total.value / PAGE_SIZE)),
);

const {
  posts,
  total,
  loading,
  error,
  retry: loadMyPosts,
} = usePostListQuery('mine', () => ({ page: page.value, pageSize: PAGE_SIZE }));

function changePage(nextPage: number) {
  void router.push({
    name: 'my-posts',
    query: nextPage > 1 ? { page: String(nextPage) } : {},
  });
  scrollToTop({ behavior: 'smooth' });
}
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="mx-auto max-w-4xl">
      <header class="mb-5">
        <h1 class="text-2xl font-bold tracking-tight text-ink">我的帖子</h1>
        <p class="mt-1 text-sm text-muted">查看你发表过的帖子。</p>
      </header>

      <section v-if="!authUser" class="py-8 text-center">
        <div
          class="mx-auto grid size-12 place-items-center rounded-full bg-primary-soft text-primary"
          aria-hidden="true"
        >
          <ArticleOutlined class="size-6" />
        </div>
        <h2 class="mt-4 text-lg font-semibold text-ink">登录后查看帖子</h2>
        <p class="mt-2 text-sm text-muted">
          请使用页面右上角的登录入口登录账号。
        </p>
      </section>

      <PostList
        v-else
        :posts="posts"
        :loading="loading"
        :error="error"
        :page="page"
        :total-pages="totalPages"
        empty-title="还没有发表帖子"
        empty-description="发表帖子后，它们会出现在这里。"
        @retry="loadMyPosts"
        @change-page="changePage"
      />
    </div>
  </div>
</template>
