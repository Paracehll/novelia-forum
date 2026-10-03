<script setup lang="ts">
import { InfoOutlined } from '@vicons/material';
import { RouterLink } from 'vue-router';
</script>

<template>
  <div
    class="flex items-start gap-2 rounded-md border border-primary/15 bg-primary-soft px-3 py-2 text-sm text-ink"
  >
    <InfoOutlined
      class="mt-0.5 size-4 shrink-0 text-primary"
      aria-hidden="true"
    />
    <ul class="list-disc space-y-1 pl-4 leading-relaxed">
      <li>
        提问前先阅读
        <RouterLink
          :to="{ name: 'posts', params: { slug: 'announcements' } }"
          class="font-semibold text-primary underline underline-offset-2 hover:text-primary-hover"
        >
          站务公告
        </RouterLink>
        的
        <strong>新人教程</strong>
        ，再搜索
        <RouterLink
          :to="{ name: 'posts', params: { slug: 'feedback' } }"
          class="font-semibold text-primary underline underline-offset-2 hover:text-primary-hover"
        >
          意见反馈
        </RouterLink>
        中的相同案例，避免重复发帖。
      </li>
      <li>报错请详述问题，并附小说链接或截图。</li>
      <li>
        求书请前往
        <RouterLink
          :to="{
            name: 'posts',
            params: { slug: 'novel' },
            query: { q: '求书集中贴' },
          }"
          class="font-semibold text-primary underline underline-offset-2 hover:text-primary-hover"
        >
          求书集中贴
        </RouterLink>
        ，请勿单独发帖。
      </li>
      <li>
        碰到此类建政小说时，不要在论坛发帖或群里讨论，请通过 QQ 或 Telegram
        私信管理员处理。
      </li>
      <li>
        发言请遵守
        <RouterLink
          :to="{ name: 'community-rules' }"
          class="font-semibold text-primary underline underline-offset-2 hover:text-primary-hover"
        >
          社区守则
        </RouterLink>
        ，累计三次违规即出局。
      </li>
    </ul>
  </div>
</template>
