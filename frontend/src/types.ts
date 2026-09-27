// Bu tipler, backend'deki internal/domain/content.go içindeki
// Content ve ScoredContent struct'larının JSON tag'leriyle BİREBİR eşleşir.
// API sözleşmesi değişirse (yeni alan, yeniden adlandırma), önce backend'deki
// domain paketine, sonra buraya bakılır — tek doğruluk kaynağı backend'dir.

export type ContentType = "video" | "text";

export interface ScoredContent {
  external_id: string;
  provider: string;
  title: string;
  type: ContentType;
  views: number;
  likes: number;
  reading_time_minutes: number;
  reactions: number;
  published_at: string;
  fetched_at: string;
  raw_fields?: Record<string, unknown>;
  // scoring.Score formülünün terimleri — bkz. internal/scoring/scoring.go.
  // Not: final_score dışındakiler SINIRSIZ olabilir (0-1 aralığına
  // normalize edilmiş DEĞİLDİR), bu yüzden UI tarafında bunları yüzdelik
  // bir bar yerine mevcut sonuç kümesindeki MAKSİMUMA göre orantılıyoruz.
  base_score: number;
  freshness_score: number;
  engagement_score: number;
  final_score: number;
}

export type SortField = "final_score" | "base_score" | "engagement_score" | "freshness_score";

export interface SearchParams {
  q: string;
  type: ContentType | "all";
  page: number;
  pageSize: number;
}

export interface SearchResponse {
  results: ScoredContent[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}
