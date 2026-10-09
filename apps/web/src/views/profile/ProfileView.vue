<script setup lang="ts">
import {
  ArticleOutlined,
  PersonOutlineOutlined,
  StarBorderOutlined,
} from '@vicons/material';
import { useWebKit, XTime } from '@novelia/web-kit';
import { RouterLink, RouterView, useRoute } from 'vue-router';

const { whoami } = useWebKit();
const route = useRoute();
const sections = [
  { name: 'my-posts', label: '我的帖子', icon: ArticleOutlined },
  { name: 'favorites', label: '我的收藏', icon: StarBorderOutlined },
];
</script>

<template>
  <div class="page-container py-4 md:py-6">
    <div class="mx-auto max-w-4xl">
      <header class="mb-6">
        <div class="mt-4 flex items-center gap-4">
          <div
            class="grid size-14 shrink-0 place-items-center rounded-full bg-primary-soft text-xl font-semibold text-primary"
            aria-hidden="true"
          >
            <template v-if="whoami.user">
              {{ Array.from(whoami.user.username)[0] }}
            </template>
            <PersonOutlineOutlined v-else class="size-7" />
          </div>
          <div class="min-w-0">
            <p class="break-words text-lg font-semibold text-ink">
              {{ whoami.user?.username ?? '尚未登录' }}
            </p>
            <p v-if="whoami.user" class="mt-1 text-sm text-muted">
              <XTime :time="whoami.user.createdAt" preset="date" />
            </p>
          </div>
        </div>
      </header>

      <nav
        class="mb-5 flex gap-6 border-b border-divider"
        aria-label="个人主页栏目"
      >
        <RouterLink
          v-for="section in sections"
          :key="section.name"
          :to="{ name: section.name }"
          :aria-current="route.name === section.name ? 'page' : undefined"
          class="flex items-center gap-2 border-b-2 px-1 pb-3 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary"
          :class="
            route.name === section.name
              ? 'border-primary text-primary'
              : 'border-transparent text-muted hover:text-ink'
          "
        >
          <component :is="section.icon" class="size-5" aria-hidden="true" />
          {{ section.label }}
        </RouterLink>
      </nav>

      <RouterView />
    </div>
  </div>
</template>
