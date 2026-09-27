import type { ContentType, SortField } from "../types";

interface FilterBarProps {
  type: ContentType | "all";
  onTypeChange: (type: ContentType | "all") => void;
  sort: SortField;
  onSortChange: (sort: SortField) => void;
}

const SORT_LABELS: Record<SortField, string> = {
  final_score: "Genel skor",
  base_score: "Temel Puan",
  engagement_score: "Etkileşim",
  freshness_score: "Güncellik",
};

// Tek bir "segmented" araç çubuğu: kenarlık + cam zemin tonu (bg-white/[0.03],
// border-white/10) sonuç kartlarıyla birebir aynı — ayrık iki kutu yerine tek
// bütün bir parça olarak arama kutusunun hemen altında oturuyor. Vurgu rengi
// (hover/focus) marka mavisi, arama kutusundaki parlamayla aynı aileden.
const selectClass =
  "appearance-none rounded-xl bg-transparent py-2 pl-3 pr-8 text-sm text-neutral-200 outline-none transition-colors hover:bg-brand-secondary/10 focus:bg-brand-secondary/10";

function Chevron() {
  return (
    <svg
      className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-neutral-500"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
    >
      <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function FilterBar({ type, onTypeChange, sort, onSortChange }: FilterBarProps) {
  return (
    <div className="inline-flex items-center gap-0.5 rounded-2xl border border-white/10 bg-white/[0.03] p-1 backdrop-blur-sm">
      <div className="relative">
        <select
          value={type}
          onChange={(e) => onTypeChange(e.target.value as ContentType | "all")}
          className={selectClass}
        >
          <option value="all">Tüm türler</option>
          <option value="video">Video</option>
          <option value="text">Metin</option>
        </select>
        <Chevron />
      </div>

      <div className="h-5 w-px shrink-0 bg-white/10" />

      <div className="relative">
        <select value={sort} onChange={(e) => onSortChange(e.target.value as SortField)} className={selectClass}>
          {Object.entries(SORT_LABELS).map(([value, label]) => (
            <option key={value} value={value}>
              Sırala: {label}
            </option>
          ))}
        </select>
        <Chevron />
      </div>
    </div>
  );
}
