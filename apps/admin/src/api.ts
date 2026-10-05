import { inject, type InjectionKey } from 'vue';

import type { AdminKitContext } from '@novelia/admin-kit';

type AuthApi = AdminKitContext['api'];

export interface Page<T> {
  total: number;
  items: T[];
}

export type PostStatus = 0 | 1 | 2;
export type CommentStatus = 0 | 1 | 2;
export type CommentStatusName = 'published' | 'hidden' | 'deleted';
export type PostStatusFilter = 'all' | '0' | '1' | '2';
export type CommentStatusFilter = 'all' | '0' | '1' | '2';

export interface Category {
  id: number;
  slug: string;
}

export interface CategoryTag {
  id: number;
  name: string;
  color: number;
  sortOrder: number;
}

export interface CategoryListItem extends Category {
  tags: CategoryTag[];
}

export type PostSort = 'active' | 'newest' | 'views' | 'comments';

export interface Tag {
  id: number;
  name: string;
  color: number;
  isActive: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
}

export interface PostTag {
  id: number;
  name: string;
  color: number;
}

export interface PostSummary {
  id: number;
  categoryId: number;
  title: string;
  authorId: number;
  authorUsername: string;
  status: PostStatus;
  viewsCount: number;
  commentsCount: number;
  commentsLocked: boolean;
  pinOrder: number | null;
  favorited: boolean;
  createdAt: string;
  updatedAt: string;
  activeAt: string;
  tags: PostTag[];
}

export interface Comment {
  id: number;
  postId: number;
  rootId: number | null;
  content: string;
  authorId: number;
  authorUsername: string;
  status: CommentStatus;
  createdAt: string;
  updatedAt: string;
}

export interface CommentListParams {
  page: number;
  pageSize: number;
  query?: string;
  authorName?: string;
  postId?: number | null;
  status?: CommentStatusFilter;
}

interface TagRequest {
  name: string;
  color: number;
  sortOrder: number;
}

interface PostListParams {
  page: number;
  pageSize: number;
  query?: string;
  category?: string;
  status?: PostStatusFilter;
  tagId?: number | null;
  authorId?: number | null;
  authorName?: string;
  sort?: PostSort;
}

const TAG_CACHE_MAX_AGE = 60 * 60 * 1000;
const TAG_CACHE_KEY_PREFIX = 'forum:admin:tags:v1:';

interface TagCache {
  fetchedAt: number;
  items: Tag[];
}

function tagCacheKey(categoryId: number) {
  return `${TAG_CACHE_KEY_PREFIX}${categoryId}`;
}

function readTagCache(categoryId: number): Tag[] | undefined {
  try {
    const value = JSON.parse(
      localStorage.getItem(tagCacheKey(categoryId)) ?? 'null',
    ) as TagCache | null;
    if (
      !value ||
      !Number.isFinite(value.fetchedAt) ||
      value.fetchedAt <= 0 ||
      Date.now() - value.fetchedAt > TAG_CACHE_MAX_AGE ||
      value.fetchedAt > Date.now() ||
      !Array.isArray(value.items) ||
      !value.items.every(
        (tag) =>
          tag &&
          Number.isSafeInteger(tag.id) &&
          typeof tag.name === 'string' &&
          Number.isFinite(tag.color) &&
          typeof tag.isActive === 'boolean' &&
          Number.isFinite(tag.sortOrder),
      )
    )
      return;
    return value.items;
  } catch {
    // Storage is optional; fall back to the API.
  }
}

function writeTagCache(categoryId: number, items: Tag[]) {
  try {
    localStorage.setItem(
      tagCacheKey(categoryId),
      JSON.stringify({ fetchedAt: Date.now(), items } satisfies TagCache),
    );
  } catch {
    // Storage is optional; keep the fetched result for this request.
  }
}

function clearTagCache(categoryId: number) {
  try {
    localStorage.removeItem(tagCacheKey(categoryId));
  } catch {
    // A blocked storage API does not affect the write request.
  }
}

function endpoint(path: string) {
  return new URL(path, new URL('/api/v1/', window.location.origin));
}

export function createForumApi(authApi: AuthApi) {
  const client = authApi.createClient(
    new URL('/api/v1/', window.location.origin).toString(),
  );

  return {
    getCategories() {
      return client.get(endpoint('category/')).json<CategoryListItem[]>();
    },
    async getTags(categoryId: number) {
      const cached = readTagCache(categoryId);
      if (cached) return cached;
      const tags = await client
        .get(endpoint(`admin/category/${categoryId}/tag`))
        .json<Tag[]>();
      writeTagCache(categoryId, tags);
      return tags;
    },
    async createTag(categoryId: number, request: TagRequest) {
      const tag = await client
        .post(endpoint(`admin/category/${categoryId}/tag`), { json: request })
        .json<Tag>();
      clearTagCache(categoryId);
      return tag;
    },
    async updateTag(categoryId: number, id: number, request: TagRequest) {
      const tag = await client
        .put(endpoint(`admin/category/${categoryId}/tag/${id}`), {
          json: request,
        })
        .json<Tag>();
      clearTagCache(categoryId);
      return tag;
    },
    async activateTag(categoryId: number, id: number) {
      await client.put(
        endpoint(`admin/category/${categoryId}/tag/${id}/active`),
      );
      clearTagCache(categoryId);
    },
    async deactivateTag(categoryId: number, id: number) {
      await client.delete(
        endpoint(`admin/category/${categoryId}/tag/${id}/active`),
      );
      clearTagCache(categoryId);
    },
    getPosts(params: PostListParams) {
      return client
        .get(endpoint('admin/post/'), {
          searchParams: {
            page: params.page,
            page_size: params.pageSize,
            q: params.query || undefined,
            category: params.category || undefined,
            status: params.status || undefined,
            tag: params.tagId ?? undefined,
            author_id: params.authorId ?? undefined,
            author_name: params.authorName || undefined,
            sort: params.sort,
          },
        })
        .json<Page<PostSummary>>();
    },
    async setPostStatus(id: number, status: PostStatus) {
      await client.put(endpoint(`admin/post/${id}/status`), {
        json: { status },
      });
    },
    async lockPost(id: number) {
      await client.put(endpoint(`admin/post/${id}/lock`));
    },
    async unlockPost(id: number) {
      await client.delete(endpoint(`admin/post/${id}/lock`));
    },
    async pinPost(id: number, pinOrder: number) {
      await client.put(endpoint(`admin/post/${id}/pin`), {
        json: { pinOrder },
      });
    },
    async unpinPost(id: number) {
      await client.delete(endpoint(`admin/post/${id}/pin`));
    },
    getComments(params: CommentListParams) {
      return client
        .get(endpoint('admin/comment/'), {
          searchParams: {
            page: params.page,
            page_size: params.pageSize,
            q: params.query || undefined,
            author_name: params.authorName || undefined,
            post_id: params.postId ?? undefined,
            status: params.status || undefined,
          },
        })
        .json<Page<Comment>>();
    },
    async setCommentStatus(id: number, status: CommentStatusName) {
      await client.put(endpoint(`admin/comment/${id}/status`), {
        json: { status },
      });
    },
  };
}

export type ForumApi = ReturnType<typeof createForumApi>;

export const forumApiKey: InjectionKey<ForumApi> = Symbol('forum-api');

export function useForumApi() {
  const api = inject(forumApiKey);
  if (!api) throw new Error('Forum API 尚未注册');
  return api;
}
