<script setup lang="ts">
import { XTime } from '@novelia/web-kit';
import {
  ChatBubbleOutlineOutlined,
  LockOutlined,
  PushPinOutlined,
  VisibilityOutlined,
} from '@vicons/material';
import { RouterLink } from 'vue-router';

import type { PostSummary } from '@/api';
import PostTagList from '@/components/PostTagList.vue';

defineProps<{
  post: PostSummary;
}>();

const countFormatter = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 1,
});

function formatCount(value: number) {
  return countFormatter.format(value).toLowerCase();
}
</script>

<template>
  <article
    class="group relative py-2.5 transition-colors duration-300 hover:bg-primary/5 sm:grid sm:grid-cols-[minmax(0,1fr)_6rem_6rem] sm:items-center sm:gap-x-4"
  >
    <div class="min-w-0">
      <h2 class="text-base leading-snug font-medium wrap-anywhere">
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
        class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted"
      >
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <PostTagList :tags="post.tags" />
          <span class="font-medium text-ink/80">{{ post.authorUsername }}</span>
          <span>
            发布于
            <XTime :time="post.createdAt" preset="relative" />
          </span>
        </div>
        <div
          class="ml-auto flex shrink-0 items-center gap-3 sm:hidden"
          aria-label="帖子数据"
        >
          <span class="inline-flex items-center gap-1">
            <VisibilityOutlined class="size-4" aria-hidden="true" />
            <span :class="{ 'text-orange-600': post.viewsCount > 1000 }">
              {{ formatCount(post.viewsCount) }}
            </span>
          </span>
          <span class="inline-flex items-center gap-1">
            <ChatBubbleOutlineOutlined class="size-4" aria-hidden="true" />
            <span :class="{ 'text-orange-600': post.commentsCount > 1000 }">
              {{ formatCount(post.commentsCount) }}
            </span>
          </span>
        </div>
      </div>
    </div>

    <dl class="hidden text-center sm:block">
      <dt class="sr-only">查看</dt>
      <dd
        class="text-lg leading-tight font-medium tabular-nums break-all"
        :class="post.viewsCount > 1000 ? 'text-orange-600' : 'text-ink/80'"
      >
        {{ formatCount(post.viewsCount) }}
      </dd>
    </dl>
    <dl class="hidden text-center sm:block">
      <dt class="sr-only">评论</dt>
      <dd
        class="text-lg leading-tight font-medium tabular-nums break-all"
        :class="post.commentsCount > 1000 ? 'text-orange-600' : 'text-ink/80'"
      >
        {{ formatCount(post.commentsCount) }}
      </dd>
    </dl>
  </article>
</template>
