<script setup lang="ts">
import { AddOutlined } from '@vicons/material';
import { computed } from 'vue';
import { RouterLink, useRoute, useRouter } from 'vue-router';

import { type PostSort } from '@/api';
import { useWebKitLayout, XButton } from '@novelia/web-kit';
import { useCategoryStore } from '@/stores/category';
import { usePostListQuery } from '@/stores/post';
import PostFilters from './PostFilters.vue';
import PostList from './PostList.vue';

const PAGE_SIZE = 20;
const POST_SORTS = new Set<PostSort>([
  'active',
  'newest',
  'updated',
  'views',
  'comments',
]);

const route = useRoute();
const router = useRouter();
const { scrollToTop } = useWebKitLayout();
const categoryStore = useCategoryStore();

const selectedCategory = computed(() => {
  const value = route.params.slug;
  return (
    categoryStore.categories.find((category) => category.slug === value)
      ?.slug ?? categoryStore.defaultCategory.slug
  );
});

const selectedCategoryItem = computed(
  () =>
    categoryStore.categories.find(
      (category) => category.slug === selectedCategory.value,
    ) ?? categoryStore.defaultCategory,
);

const searchQuery = computed(() => {
  const value = route.query.q;
  return typeof value === 'string' ? value.trim() : '';
});

const selectedTagId = computed(() => {
  const value = Number(route.query.tag);
  return Number.isSafeInteger(value) && value > 0 ? value : undefined;
});

const selectedSort = computed<PostSort>(() => {
  const value = route.query.sort;
  return typeof value === 'string' && POST_SORTS.has(value as PostSort)
    ? (value as PostSort)
    : 'active';
});

const page = computed(() => {
  const value = Number(route.query.page);
  return Number.isInteger(value) && value > 0 ? value : 1;
});

const totalPages = computed(() =>
  Math.max(1, Math.ceil(total.value / PAGE_SIZE)),
);
const hasFilters = computed(() =>
  Boolean(searchQuery.value || selectedTagId.value),
);

const {
  posts,
  total,
  loading: postsLoading,
  error: postsError,
  retry: loadPosts,
} = usePostListQuery('category', () => ({
  page: page.value,
  pageSize: PAGE_SIZE,
  category: selectedCategory.value,
  query: searchQuery.value || undefined,
  tagIds: selectedTagId.value ? [selectedTagId.value] : undefined,
  sort: selectedSort.value,
}));

function applyFilters(filters: {
  query: string;
  tagId?: number;
  sort: PostSort;
}) {
  void router.push({
    name: 'posts',
    params: { slug: selectedCategory.value },
    query: {
      ...(filters.query ? { q: filters.query } : {}),
      ...(filters.tagId ? { tag: String(filters.tagId) } : {}),
      ...(filters.sort !== 'active' ? { sort: filters.sort } : {}),
    },
  });
}

function changePage(nextPage: number) {
  void router.push({
    name: 'posts',
    params: { slug: selectedCategory.value },
    query: {
      ...(searchQuery.value ? { q: searchQuery.value } : {}),
      ...(selectedTagId.value ? { tag: String(selectedTagId.value) } : {}),
      ...(selectedSort.value !== 'active' ? { sort: selectedSort.value } : {}),
      ...(nextPage > 1 ? { page: String(nextPage) } : {}),
    },
  });
  scrollToTop({ behavior: 'smooth' });
}
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="min-w-0 space-y-4">
      <header>
        <h1 class="text-2xl font-bold tracking-tight text-ink">
          {{ selectedCategoryItem.title }}
        </h1>
      </header>

      <aside
        v-if="selectedCategory === 'feedback'"
        aria-label="意见反馈告示"
        class="space-y-2 text-sm leading-relaxed text-ink"
      >
        <p class="text-warning">
          FishHawk陷入加班地狱，网站开发速度大幅下降已成常态，论坛反馈目前没有精力维护，有问题加群@吧
        </p>
        <p class="flex flex-wrap gap-x-4 gap-y-2">
          <a
            href="http://qm.qq.com/cgi-bin/qm/qr?_wv=1027&k=Qa0SOMBYZoJZ4vuykz3MbPS0zbpeN0pW&authKey=q75E7fr5CIBSDhqX%2F4kuC%2B0mcPiDvj%2FSDfP%2FGZ8Rl8kDn6Z3M6XPSZ91yt4ZWonq&noverify=0&group_code=819513328"
            target="_blank"
            rel="noopener noreferrer"
            class="text-primary underline underline-offset-4"
          >
            QQ群：819513328
          </a>
          <a
            href="https://t.me/+hgUSCmuBReQzNmU1"
            target="_blank"
            rel="noopener noreferrer"
            class="break-all text-primary underline underline-offset-4"
          >
            TG群
          </a>
        </p>
      </aside>
      <aside
        aria-label="新论坛问题反馈提醒"
        class="space-y-2 text-sm leading-relaxed text-ink"
      >
        <p class="text-warning">
          新论坛已上线，使用中遇到问题欢迎到
          <RouterLink
            :to="{ name: 'post-detail', params: { id: 1099 } }"
            class="text-primary underline underline-offset-4"
          >
            《新论坛问题集中反馈贴》
          </RouterLink>
          集中反馈。
        </p>
      </aside>
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
        <PostFilters
          class="order-2 lg:order-1 lg:flex-1"
          :category-id="selectedCategoryItem.id"
          :query="searchQuery"
          :tag-id="selectedTagId"
          :sort="selectedSort"
          @apply="applyFilters"
        />
        <XButton
          v-if="categoryStore.canPublish(selectedCategory)"
          class="order-1 self-end lg:order-2 lg:self-auto"
          :as="RouterLink"
          :to="{
            name: 'post-create',
            query: { category: selectedCategory },
          }"
        >
          <AddOutlined class="size-4" aria-hidden="true" />
          发表帖子
        </XButton>
      </div>
      <PostList
        :posts="posts"
        :loading="postsLoading"
        :error="postsError"
        :page="page"
        :total-pages="totalPages"
        :empty-title="hasFilters ? '没有找到帖子' : undefined"
        :empty-description="
          hasFilters ? '可以尝试调整关键词或标签条件。' : undefined
        "
        @retry="loadPosts"
        @change-page="changePage"
      />
    </div>
  </div>
</template>
