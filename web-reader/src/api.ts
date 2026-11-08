// 阅读端 API 封装：原生 fetch，数据走 TanStack Query。
// 契约见 docs/CONTRACTS.md；book_id / chapter_id 全程 string。

export interface SearchItem {
  book_id: string;
  title: string;
  author: string;
  category: string;
  abstract: string;
  thumb_url: string;
}

export interface SearchResult {
  items: SearchItem[];
  page: number;
  has_more: boolean;
  next_offset: number;
}

export interface BookDetail {
  book_id: string;
  title: string;
  author: string;
  word_number: number;
  last_chapter: string;
  intro: string;
  thumb_url: string;
}

export interface CatalogChapter {
  chapter_id: string;
  title: string;
  volume: string;
  is_vip: boolean;
  is_pay: boolean;
  updated_at: number;
}

export interface Catalog {
  book_id: string;
  total: number;
  chapters: CatalogChapter[];
}

export interface ChapterContent {
  book_id: string;
  chapter_id: string;
  title: string;
  content: string;
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "omit" });
  const body = await res.json();
  if (body.code !== 0) {
    throw new ApiError(body.code as number, body.msg as string);
  }
  return body.data as T;
}

export class ApiError extends Error {
  code: number;
  constructor(code: number, msg: string) {
    super(msg);
    this.code = code;
  }
}

export function searchBooks(q: string, page: number): Promise<SearchResult> {
  return get<SearchResult>("/api/search?q=" + encodeURIComponent(q) + "&page=" + page);
}

export function getBookDetail(bookId: string): Promise<BookDetail> {
  return get<BookDetail>("/api/detail?book_id=" + bookId);
}

export function getCatalog(bookId: string): Promise<Catalog> {
  return get<Catalog>("/api/catalog?book_id=" + bookId);
}

export function getChapterContent(bookId: string, chapterId: string): Promise<ChapterContent> {
  return get<ChapterContent>("/api/content?book_id=" + bookId + "&chapter_id=" + chapterId);
}
