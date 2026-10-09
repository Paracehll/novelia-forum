<script setup lang="ts">
import {
  PersonOutlineOutlined,
  ExploreOutlined,
  FactCheckOutlined,
  ForumOutlined,
  LanguageOutlined,
  MenuBookOutlined,
  StarBorderOutlined,
  SettingsOutlined,
} from '@vicons/material';
import {
  useWebKit,
  WebKitApp,
  WebKitLayout,
  type WebKitMenuOption,
} from '@novelia/web-kit';
import { computed, watch, type Component } from 'vue';
import { RouterView, useRoute } from 'vue-router';

import { useCategoryStore } from '@/stores/category';
import { useDraftStore } from '@/stores/draft';
import { usePostStore } from '@/stores/post';

const { whoami } = useWebKit();
const route = useRoute();
const categoryStore = useCategoryStore();
const postStore = usePostStore();
const draftStore = useDraftStore();

watch(
  () => whoami.value.user?.id,
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
    type: 'divider',
    key: 'categories-divider',
  },
  {
    type: 'group',
    key: 'light-novels',
    label: '小说站',
    icon: LanguageOutlined,
    children: [
      {
        type: 'external',
        key: 'web-novels',
        label: '网络小说',
        icon: LanguageOutlined,
        href: 'https://n.novelia.cc/novel',
      },
      {
        type: 'external',
        key: 'wenku-novels',
        label: '文库小说',
        icon: MenuBookOutlined,
        href: 'https://n.novelia.cc/wenku',
      },
      {
        type: 'external',
        key: 'novel-favorites',
        label: '我的收藏',
        icon: StarBorderOutlined,
        href: whoami.value.isSignedIn
          ? 'https://n.novelia.cc/favorite/web'
          : 'https://n.novelia.cc/favorite/local',
      },
    ],
  },
  {
    type: 'divider',
    key: 'navigation-divider',
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
    key: 'profile',
    label: '个人主页',
    icon: PersonOutlineOutlined,
    to: { name: 'profile' },
  },
  {
    key: 'account-settings',
    label: '账户设置',
    icon: SettingsOutlined,
    to: { name: 'account-settings' },
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
