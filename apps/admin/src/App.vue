<script setup lang="ts">
import {
  ArticleOutlined,
  LocalOfferOutlined,
  CommentOutlined,
  DashboardOutlined,
} from '@vicons/material';
import {
  AdminKitApp,
  AdminKitLayout,
  type AdminKitMenuOption,
} from '@novelia/admin-kit';
import { NIcon } from 'naive-ui';
import { h, type Component } from 'vue';
import { RouterView, useRoute } from 'vue-router';

const route = useRoute();

function renderIcon(icon: Component) {
  return () => h(NIcon, null, { default: () => h(icon) });
}

const menuOptions: AdminKitMenuOption[] = [
  {
    label: '概览',
    key: 'overview',
    to: { name: 'overview' },
    icon: renderIcon(DashboardOutlined),
  },
  {
    label: '标签管理',
    key: 'categories',
    to: { name: 'categories' },
    icon: renderIcon(LocalOfferOutlined),
  },
  {
    label: '帖子管理',
    key: 'posts',
    to: { name: 'posts' },
    icon: renderIcon(ArticleOutlined),
  },
  {
    label: '评论管理',
    key: 'comments',
    to: { name: 'comments' },
    icon: renderIcon(CommentOutlined),
  },
];
</script>

<template>
  <AdminKitApp>
    <AdminKitLayout
      v-if="route.meta.requiresAuth"
      :menu-options="menuOptions"
    />
    <RouterView v-else />
  </AdminKitApp>
</template>
