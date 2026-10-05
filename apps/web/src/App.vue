<script setup lang="ts">
import {
  ArticleOutlined,
  ExploreOutlined,
  FactCheckOutlined,
  ForumOutlined,
  MenuBookOutlined,
  StarBorderOutlined,
} from '@vicons/material';
import {
  WebKitApp,
  WebKitLayout,
  type WebKitMenuOption,
} from '@novelia/web-kit';
import { computed, watch, type Component } from 'vue';
import { RouterView, useRoute } from 'vue-router';

import { authUser } from '@/api';
import { useCategoryStore } from '@/stores/category';
import { useDraftStore } from '@/stores/draft';
import { usePostStore } from '@/stores/post';

const route = useRoute();
const categoryStore = useCategoryStore();
const postStore = usePostStore();
const draftStore = useDraftStore();

watch(
  () => authUser.value?.id,
  (userId, previousUserId) => {
    if (previousUserId && userId !== previousUserId) {
      draftStore.clearAllDrafts();
      draftStore.$persist();
    }
  },
  { flush: 'sync' },
);

function categoryIcon(slug: string): Component {
  if (slug === 'novel') return MenuBookOutlined;
  if (slug === 'announcements') return ExploreOutlined;
  return ForumOutlined;
}

const navigationOptions = computed<WebKitMenuOption[]>(() => [
  ...categoryStore.categories.map((category) => ({
    key: category.slug,
    label: category.title,
    icon: categoryIcon(category.slug),
    to: { name: 'posts', params: { slug: category.slug } },
  })),
  {
    key: 'light-novels',
    label: '轻小说机翻站',
    icon: MenuBookOutlined,
    to: { name: 'light-novels' },
  },
  {
    key: 'community-rules',
    label: '社区守则',
    icon: FactCheckOutlined,
    to: { name: 'community-rules' },
  },
]);

const accountOptions: WebKitMenuOption[] = [
  {
    key: 'my-posts',
    label: '我的帖子',
    icon: ArticleOutlined,
    to: { name: 'my-posts' },
  },
  {
    key: 'favorites',
    label: '我的收藏',
    icon: StarBorderOutlined,
    to: { name: 'favorites' },
  },
];

const selectedNavigationKey = computed(() => {
  if (route.name === 'community-rules') return 'community-rules';

  if (route.name === 'posts' && typeof route.params.slug === 'string') {
    return route.params.slug;
  }

  if (route.name === 'post-detail') {
    return categoryStore.categories.find(
      (category) => category.id === postStore.currentPost?.categoryId,
    )?.slug;
  }

  return undefined;
});
</script>

<template>
  <WebKitApp>
    <WebKitLayout
      :navigation-options="navigationOptions"
      :account-options="accountOptions"
      :selected-navigation-key="selectedNavigationKey"
    >
      <RouterView />
    </WebKitLayout>
  </WebKitApp>
</template>
