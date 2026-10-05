import { ref } from 'vue';
import { defineStore } from 'pinia';

export interface PostDraft {
  title: string;
  category: string;
  tagIds: number[];
  content: string;
}

const MAX_DRAFT_COUNT = 20;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function deserializeDrafts(value: string) {
  const state: unknown = JSON.parse(value);
  if (
    !isRecord(state) ||
    !isRecord(state.postDrafts) ||
    !isRecord(state.commentDrafts) ||
    !isRecord(state.draftUpdatedAt) ||
    !Object.values(state.postDrafts).every(
      (draft) =>
        isRecord(draft) &&
        typeof draft.title === 'string' &&
        typeof draft.category === 'string' &&
        typeof draft.content === 'string' &&
        Array.isArray(draft.tagIds) &&
        draft.tagIds.every((id) => Number.isSafeInteger(id) && id > 0),
    ) ||
    !Object.values(state.commentDrafts).every(
      (content) => typeof content === 'string',
    ) ||
    !Object.values(state.draftUpdatedAt).every(
      (timestamp) =>
        typeof timestamp === 'number' && Number.isFinite(timestamp),
    )
  ) {
    // The persistence plugin catches this and keeps the empty initial state.
    throw new Error('Invalid draft cache');
  }
  return {
    postDrafts: state.postDrafts,
    commentDrafts: state.commentDrafts,
    draftUpdatedAt: state.draftUpdatedAt,
  };
}

export const useDraftStore = defineStore(
  'draft',
  () => {
    const postDrafts = ref<Record<number, PostDraft>>({});
    const commentDrafts = ref<Record<string, string>>({});
    const draftUpdatedAt = ref<Record<string, number>>({});

    function postTimestampKey(userId: number | string) {
      return `post:${userId}`;
    }

    function commentTimestampKey(key: string) {
      return `comment:${key}`;
    }

    function trimDrafts() {
      const entries = [
        ...Object.keys(postDrafts.value).map((key) => ({
          type: 'post' as const,
          key,
          updatedAt: draftUpdatedAt.value[postTimestampKey(key)] ?? 0,
        })),
        ...Object.keys(commentDrafts.value).map((key) => ({
          type: 'comment' as const,
          key,
          updatedAt: draftUpdatedAt.value[commentTimestampKey(key)] ?? 0,
        })),
      ].sort((left, right) => left.updatedAt - right.updatedAt);

      for (const entry of entries.slice(0, -MAX_DRAFT_COUNT)) {
        if (entry.type === 'post') delete postDrafts.value[Number(entry.key)];
        else delete commentDrafts.value[entry.key];
        delete draftUpdatedAt.value[
          entry.type === 'post'
            ? postTimestampKey(entry.key)
            : commentTimestampKey(entry.key)
        ];
      }
    }

    function getPostDraft(userId: number) {
      trimDrafts();
      const draft = postDrafts.value[userId];
      if (!draft) return;
      return { ...draft, tagIds: [...draft.tagIds] };
    }

    function savePostDraft(userId: number, draft: PostDraft) {
      if (!draft.title.trim() && !draft.content.trim()) {
        clearPostDraft(userId);
        return;
      }
      postDrafts.value[userId] = { ...draft, tagIds: [...draft.tagIds] };
      draftUpdatedAt.value[postTimestampKey(userId)] = Date.now();
      trimDrafts();
    }

    function clearPostDraft(userId: number) {
      delete postDrafts.value[userId];
      delete draftUpdatedAt.value[postTimestampKey(userId)];
    }

    function getCommentDraft(key: string) {
      trimDrafts();
      return commentDrafts.value[key] ?? '';
    }

    function saveCommentDraft(key: string, content: string) {
      if (!content.trim()) {
        clearCommentDraft(key);
        return;
      }
      commentDrafts.value[key] = content;
      draftUpdatedAt.value[commentTimestampKey(key)] = Date.now();
      trimDrafts();
    }

    function clearCommentDraft(key: string) {
      delete commentDrafts.value[key];
      delete draftUpdatedAt.value[commentTimestampKey(key)];
    }

    function clearAllDrafts() {
      postDrafts.value = {};
      commentDrafts.value = {};
      draftUpdatedAt.value = {};
    }

    return {
      clearAllDrafts,
      clearCommentDraft,
      clearPostDraft,
      commentDrafts,
      draftUpdatedAt,
      getCommentDraft,
      getPostDraft,
      postDrafts,
      saveCommentDraft,
      savePostDraft,
    };
  },
  {
    persist: {
      key: 'forum:drafts:v1',
      // Access storage inside the plugin's error handling, not at module load.
      storage: {
        getItem: (key) => localStorage.getItem(key),
        setItem: (key, value) => localStorage.setItem(key, value),
      },
      serializer: {
        serialize: JSON.stringify,
        deserialize: deserializeDrafts,
      },
    },
  },
);
