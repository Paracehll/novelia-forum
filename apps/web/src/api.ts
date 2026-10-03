import type { createWebKit } from '@novelia/web-kit';

type WebKit = ReturnType<typeof createWebKit>;

export let authApi: WebKit['api'];
export let authUser: WebKit['profile'];
let client: ReturnType<WebKit['api']['createClient']>;

export function initializeApi(api: WebKit['api'], profile: WebKit['profile']) {
  authApi = api;
  authUser = profile;
  client = authApi.createClient(
    new URL('/api/v1/', window.location.origin).toString(),
  );
}

export interface Page<T> {
  total: number;
  items: T[];
}

export type PostStatus = 0 | 1 | 2;
export type CommentStatus = 0 | 1 | 2;
export type CommentStatusName = 'published' | 'hidden' | 'deleted';

export interface PostTag {
  id: number;
  name: string;
  color: number;
}

export interface CategoryTag extends PostTag {
  sortOrder: number;
}

export interface CategoryListItem {
  id: number;
  slug: string;
  tags: CategoryTag[];
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

export interface Post extends PostSummary {
  content: string;
}

export type PostSort = 'active' | 'newest' | 'views' | 'comments';

export interface PostComment {
  id: number;
  postId: number;
  rootId: number | null;
  content: string;
  authorId: number;
  authorUsername: string;
  status: CommentStatus;
  createdAt: string;
  updatedAt: string;
  replyCount: number;
  replies?: Page<PostComment>;
}

export function getPosts(
  params: {
    page: number;
    pageSize: number;
    category?: string;
    query?: string;
    tagIds?: number[];
    sort?: PostSort;
  },
  signal?: AbortSignal,
) {
  return client
    .get('post/', {
      searchParams: {
        page: params.page,
        page_size: params.pageSize,
        category: params.category,
        q: params.query,
        tag: params.tagIds?.join(','),
        sort: params.sort,
      },
      signal,
    })
    .json<Page<PostSummary>>();
}

export function getFavoritePosts(
  params: { page: number; pageSize: number },
  signal?: AbortSignal,
) {
  return client
    .get('me/favorite', {
      searchParams: {
        page: params.page,
        page_size: params.pageSize,
      },
      signal,
    })
    .json<Page<PostSummary>>();
}

export function getMyPosts(
  params: { page: number; pageSize: number },
  signal?: AbortSignal,
) {
  return client
    .get('me/post', {
      searchParams: {
        page: params.page,
        page_size: params.pageSize,
      },
      signal,
    })
    .json<Page<PostSummary>>();
}

export function getPost(id: number, signal?: AbortSignal) {
  return client.get(`post/${id}/`, { signal }).json<Post>();
}

export async function setPostFavorite(id: number, favorited: boolean) {
  const path = `post/${id}/favorite`;
  await (favorited ? client.put(path) : client.delete(path));
}

export function getCategories(signal?: AbortSignal) {
  return client.get('category/', { signal }).json<CategoryListItem[]>();
}

export function createPost(input: {
  categoryId: number;
  title: string;
  content: string;
  tagIds: number[];
}) {
  return client.post('post/', { json: input }).json<Post>();
}

export function updatePost(
  id: number,
  input: {
    categoryId: number;
    title: string;
    content: string;
    tagIds: number[];
  },
) {
  return client.patch(`post/${id}/`, { json: input }).json<Post>();
}

export async function deletePost(id: number) {
  await client.delete(`post/${id}/`);
}

export async function setPostStatus(id: number, status: PostStatus) {
  await client.put(`admin/post/${id}/status`, { json: { status } });
}

export async function lockPost(id: number) {
  await client.put(`admin/post/${id}/lock`);
}

export async function unlockPost(id: number) {
  await client.delete(`admin/post/${id}/lock`);
}

export async function pinPost(id: number, pinOrder: number) {
  await client.put(`admin/post/${id}/pin`, { json: { pinOrder } });
}

export async function unpinPost(id: number) {
  await client.delete(`admin/post/${id}/pin`);
}

export function getPostComments(
  id: number,
  params: { page: number; pageSize: number },
  signal?: AbortSignal,
) {
  return client
    .get(`post/${id}/comment`, {
      searchParams: {
        page: params.page,
        page_size: params.pageSize,
      },
      signal,
    })
    .json<Page<PostComment>>();
}

export function getPostCommentReplies(
  postId: number,
  rootId: number,
  params: { page: number; pageSize: number },
  signal?: AbortSignal,
) {
  return client
    .get(`post/${postId}/comment/${rootId}/reply`, {
      searchParams: {
        page: params.page,
        page_size: params.pageSize,
      },
      signal,
    })
    .json<Page<PostComment>>();
}

export function createPostComment(
  id: number,
  input: { content: string; rootId?: number },
) {
  return client.post(`post/${id}/comment`, { json: input }).json<PostComment>();
}

export function updatePostComment(id: number, content: string) {
  return client
    .patch(`comment/${id}`, { json: { content } })
    .json<PostComment>();
}

export async function deletePostComment(id: number) {
  await client.delete(`comment/${id}`);
}

export async function setPostCommentStatus(
  id: number,
  status: CommentStatusName,
) {
  await client.put(`admin/comment/${id}/status`, { json: { status } });
}

export async function deleteCommentsByAuthor(authorId: number) {
  await client.delete(`admin/comment/author/${authorId}`);
}
