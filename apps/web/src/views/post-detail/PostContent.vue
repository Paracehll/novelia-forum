<script setup lang="ts">
import { computed } from 'vue';

import type { Post } from '@/api';
import MarkdownContent from '@/components/markdown/MarkdownContent.vue';

const props = defineProps<{
  post: Post;
}>();

const hasBeenUpdated = computed(
  () =>
    new Date(props.post.updatedAt).getTime() >
    new Date(props.post.createdAt).getTime(),
);

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}
</script>

<template>
  <article class="pb-6">
    <h1
      class="text-2xl leading-tight font-bold tracking-tight text-ink sm:text-3xl"
    >
      {{ post.title }}
    </h1>

    <div
      class="mt-5 flex flex-wrap items-center justify-between gap-3 text-xs text-muted"
    >
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span class="font-medium text-ink/80">{{ post.authorUsername }}</span>
        <span aria-hidden="true">·</span>
        <span>
          发布于
          <time :datetime="post.createdAt">
            {{ formatDate(post.createdAt) }}
          </time>
        </span>
        <template v-if="hasBeenUpdated">
          <span aria-hidden="true">·</span>
          <span>
            更新于
            <time :datetime="post.updatedAt">
              {{ formatDate(post.updatedAt) }}
            </time>
          </span>
        </template>
      </div>
      <div class="flex items-center gap-4" aria-label="帖子数据">
        <span>{{ post.viewsCount }} 次浏览</span>
        <span>{{ post.commentsCount }} 条评论</span>
      </div>
    </div>

    <div v-if="$slots.actions" class="mt-4">
      <slot name="actions" />
    </div>
    <div class="my-6 h-px bg-divider" />
    <MarkdownContent mode="article" :source="post.content" />
  </article>
</template>
