create table if not exists tag
(
    id           bigint generated always as identity primary key,
    category_id  bigint       not null,
    name         varchar(64)  not null,
    color        smallint     not null,
    is_active    boolean      not null default true,
    sort_order   int          not null,
    created_at   timestamptz  not null default current_timestamp,
    updated_at   timestamptz  not null default current_timestamp,
    attr         jsonb        not null default '{}'::jsonb,
    unique (category_id, name)
);

comment on column tag.color is '预定义颜色的编号';
comment on column tag.is_active is 'true=可用于新帖绑定；false=停止新增绑定';
comment on column tag.sort_order is '标签展示顺序，数值越小越靠前';

create table if not exists post
(
    id                 bigint generated always as identity primary key,
    category_id        bigint       not null,
    title              varchar(500) not null,
    author_id          bigint       not null,
    author_username    varchar(128) not null,
    content            text         not null,
    status             smallint     not null default 0,
    views_count        int          not null default 0,
    comments_count     int          not null default 0,
    comments_locked    boolean      not null default false,
    pin_order          int,
    created_at         timestamptz  not null default current_timestamp,
    updated_at         timestamptz  not null default current_timestamp,
    active_at          timestamptz  not null default current_timestamp,
    attr               jsonb        not null default '{}'::jsonb
);

comment on column post.status is '帖子状态: 0=published, 1=hidden, 2=deleted';
comment on column post.views_count is '帖子详情数据成功读取后的累计次数，每次获取计 1 次；尽力统计，计数失败不影响读取，不代表浏览器完成渲染或独立访客数';
comment on column post.comments_count is '帖子下 status=published 的评论数量，统计一级评论和子回复';
comment on column post.comments_locked is 'true=评论区锁定状态，禁止新增评论；false=评论区开放状态，可新增评论';
comment on column post.pin_order is 'NULL=普通帖；非NULL=置顶帖的排序优先级，数值越小越靠前';
comment on column post.active_at is '帖子最后活跃时间；产生有效新评论等活跃事件时由应用层更新，用于列表排序';

create index if not exists idx_post_published_pin_active
    on post (pin_order nulls last, active_at desc, id desc)
    where status = 0;

create index if not exists idx_post_category_published_pin_active
    on post (category_id, pin_order nulls last, active_at desc, id desc)
    where status = 0;

create index if not exists idx_post_category_status_active
    on post (category_id, status, active_at desc, id desc)
    where status != 0;

create index if not exists idx_post_author_created
    on post (author_id, created_at desc, id desc);

create table if not exists post_tag
(
    post_id      bigint      not null,
    tag_id       bigint      not null,
    created_at   timestamptz not null default current_timestamp,
    primary key (post_id, tag_id)
);

create index if not exists idx_post_tag_tag_post
    on post_tag (tag_id, post_id);

create table if not exists comment
(
    id                 bigint generated always as identity primary key,
    subject_type       smallint     not null,
    subject_key        varchar(255) not null,
    root_id            bigint,
    content            text         not null,
    author_id          bigint       not null,
    author_username    varchar(128) not null,
    status             smallint     not null default 0,
    created_at         timestamptz  not null default current_timestamp,
    updated_at         timestamptz  not null default current_timestamp,
    attr               jsonb        not null default '{}'::jsonb
);

comment on column comment.status is '评论状态: 0=published, 1=hidden, 2=deleted';
comment on column comment.root_id is 'NULL=一级评论；非 NULL=子回复所属的一级根评论 ID';

create index if not exists idx_comment_subject_thread_created
    on comment (subject_type, subject_key, (coalesce(root_id, id)), created_at, id)
    where status = 0;

create index if not exists idx_comment_subject_roots_created
    on comment (subject_type, subject_key, created_at, id)
    where root_id is null;

create index if not exists idx_comment_subject_replies_created
    on comment (subject_type, subject_key, root_id, created_at, id)
    where root_id is not null;

create index if not exists idx_comment_author_created
    on comment (author_id, created_at desc, id desc);

create table if not exists post_favorite
(
    post_id       bigint       not null,
    user_id       bigint       not null,
    created_at    timestamptz  not null default current_timestamp,
    primary key (post_id, user_id)
);

create index if not exists idx_post_favorite_user_created
    on post_favorite (user_id, created_at desc, post_id desc);
