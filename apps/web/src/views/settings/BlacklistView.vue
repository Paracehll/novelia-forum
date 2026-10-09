<script setup lang="ts">
import { BlockOutlined } from '@vicons/material';
import {
  XAsyncContent,
  XButton,
  XConfirmDialog,
  XTime,
} from '@novelia/web-kit';
import { useBlacklistManagement } from '@/composables/useBlacklistManagement';

const {
  whoami,
  entries,
  loading,
  error,
  selected,
  removing,
  load,
  confirmRemove,
} = useBlacklistManagement();
</script>

<template>
  <section aria-labelledby="blacklist-heading">
    <header class="mb-4 border-b border-divider pb-3">
      <h2
        id="blacklist-heading"
        class="text-2xl font-normal tracking-tight text-ink"
      >
        黑名单管理
      </h2>
    </header>
    <p class="mb-5 text-sm leading-relaxed text-muted">
      拉黑后，帖子列表和收藏列表将不再显示该用户的帖子，评论区将隐藏其评论及其发起的评论串。
    </p>

    <div v-if="!whoami.isSignedIn" class="py-8 text-center">
      <div
        class="mx-auto grid size-12 place-items-center rounded-full bg-primary-soft text-primary"
        aria-hidden="true"
      >
        <BlockOutlined class="size-6" />
      </div>
      <h2 class="mt-4 text-lg font-semibold text-ink">登录后管理黑名单</h2>
      <p class="mt-2 text-sm text-muted">
        请使用页面右上角的登录入口登录账号。
      </p>
    </div>

    <div v-else aria-live="polite">
      <XAsyncContent
        :loading="loading"
        :error="error"
        :empty="!entries.length"
        error-title="黑名单加载失败"
        empty-title="黑名单为空"
        empty-description="你还没有拉黑任何用户。"
        @retry="load"
      >
        <p class="mb-3 text-sm text-muted">
          已拉黑 {{ entries.length }} 位用户
        </p>
        <ul
          class="divide-y divide-divider rounded-md border border-divider px-4"
        >
          <li
            v-for="entry in entries"
            :key="entry.userId"
            class="flex items-center justify-between gap-4 py-4"
          >
            <div class="min-w-0">
              <p class="break-words font-medium text-ink">
                用户 ID：{{ entry.userId }}
              </p>
              <p class="mt-1 text-xs text-muted">
                拉黑于
                <XTime :time="entry.createdAt" preset="date" />
              </p>
            </div>
            <XButton
              variant="outline"
              size="sm"
              class="shrink-0"
              :disabled="removing"
              :aria-label="`取消拉黑用户 ${entry.userId}`"
              @click="selected = entry"
            >
              取消拉黑
            </XButton>
          </li>
        </ul>
      </XAsyncContent>
    </div>

    <XConfirmDialog
      v-if="selected"
      open
      :title="`取消拉黑用户 ${selected.userId}`"
      description="取消后，该用户的帖子和评论将重新出现在列表中。"
      confirm-label="取消拉黑"
      :loading="removing"
      @update:open="
        (open) => {
          if (!open && !removing) selected = undefined;
        }
      "
      @confirm="confirmRemove"
    />
  </section>
</template>
