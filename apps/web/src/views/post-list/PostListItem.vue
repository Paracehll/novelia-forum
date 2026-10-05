<script setup lang="ts">
import {
  ChatBubbleOutlineOutlined,
  LockOutlined,
  PushPinOutlined,
  VisibilityOutlined,
} from '@vicons/material';
import { computed } from 'vue';
import { RouterLink } from 'vue-router';

import type { PostSummary } from '@/api';
import PostTagList from '@/components/PostTagList.vue';

const props = defineProps<{
  post: PostSummary;
}>();

const timestamp = computed(() => {
  const updated =
    new Date(props.post.updatedAt).getTime() >
    new Date(props.post.createdAt).getTime();
  return {
    label: updated ? '更新于' : '发布于',
    value: updated ? props.post.updatedAt : props.post.createdAt,
  };
});

const countFormatter = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 1,
});

function formatCount(value: number) {
  return countFormatter.format(value).toLowerCase();
}

function formatDate(value: string) {
  const date = new Date(value);
  const now = new Date();
  const sameYear = date.getFullYear() === now.getFullYear();
  return new Intl.DateTimeFormat('zh-CN', {
    year: sameYear ? undefined : 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
</script>

<template>
  <article
    class="group relative py-4 transition-colors duration-300 sm:grid sm:grid-cols-[minmax(0,1fr)_6rem_6rem] sm:items-center sm:gap-x-4"
  >
    <div class="min-w-0">
      <PostTagList class="mb-2" :tags="post.tags" />

      <h2 class="text-base leading-snug font-medium">
        <span
          v-if="post.pinOrder != null"
          class="mr-1.5 inline-flex size-5 items-center justify-center rounded-sm bg-orange-50 align-text-bottom text-orange-600"
          title="已置顶"
          aria-label="已置顶"
        >
          <PushPinOutlined class="size-3.5" aria-hidden="true" />
        </span>
        <span
          v-if="post.commentsLocked"
          class="mr-1.5 inline-flex size-5 items-center justify-center rounded-sm bg-orange-50 align-text-bottom text-orange-600"
          title="评论区已锁定"
          aria-label="评论区已锁定"
        >
          <LockOutlined class="size-3.5" aria-hidden="true" />
        </span>
        <RouterLink
          :to="{ name: 'post-detail', params: { id: post.id } }"
          class="text-primary transition-colors duration-300 before:absolute before:inset-0 focus-visible:rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary"
        >
          {{ post.title }}
        </RouterLink>
      </h2>

      <div
        class="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-muted"
      >
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span class="font-medium text-ink/80">{{ post.authorUsername }}</span>
          <span aria-hidden="true">·</span>
          <span>
            {{ timestamp.label }}
            <time
              :datetime="timestamp.value"
              :title="new Date(timestamp.value).toLocaleString('zh-CN')"
            >
              {{ formatDate(timestamp.value) }}
            </time>
          </span>
        </div>
        <div class="flex items-center gap-4 sm:hidden" aria-label="帖子数据">
          <span class="inline-flex items-center gap-1.5">
            <VisibilityOutlined class="size-4" aria-hidden="true" />
            <span :class="{ 'text-orange-600': post.viewsCount > 1000 }">
              {{ formatCount(post.viewsCount) }}
            </span>
          </span>
          <span class="inline-flex items-center gap-1.5">
            <ChatBubbleOutlineOutlined class="size-4" aria-hidden="true" />
            <span :class="{ 'text-orange-600': post.commentsCount > 1000 }">
              {{ formatCount(post.commentsCount) }}
            </span>
          </span>
        </div>
      </div>
    </div>

    <dl class="hidden text-center sm:block">
      <dt class="text-xs text-muted">查看</dt>
      <dd
        class="mt-1 text-lg leading-tight font-medium tabular-nums break-all"
        :class="post.viewsCount > 1000 ? 'text-orange-600' : 'text-ink/80'"
      >
        {{ formatCount(post.viewsCount) }}
      </dd>
    </dl>
    <dl class="hidden text-center sm:block">
      <dt class="text-xs text-muted">评论</dt>
      <dd
        class="mt-1 text-lg leading-tight font-medium tabular-nums break-all"
        :class="post.commentsCount > 1000 ? 'text-orange-600' : 'text-ink/80'"
      >
        {{ formatCount(post.commentsCount) }}
      </dd>
    </dl>
  </article>
</template>
