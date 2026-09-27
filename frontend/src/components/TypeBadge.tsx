import type { ContentType } from "../types";

export function TypeBadge({ type }: { type: ContentType }) {
  const isVideo = type === "video";
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${
        isVideo
          ? "border-brand-purple/30 bg-brand-purple/10 text-brand-purple"
          : "border-brand-secondary/30 bg-brand-secondary/10 text-brand-secondary"
      }`}
    >
      {isVideo ? (
        <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
          <path d="M8 5v14l11-7z" />
        </svg>
      ) : (
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M4 6h16M4 12h16M4 18h10" strokeLinecap="round" />
        </svg>
      )}
      {isVideo ? "Video" : "Metin"}
    </span>
  );
}
