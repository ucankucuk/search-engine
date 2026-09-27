export function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-white/10 py-16 text-center">
      <svg
        width="40"
        height="40"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        className="mb-3 text-neutral-600"
      >
        <circle cx="11" cy="11" r="7" />
        <path d="m21 21-4.3-4.3" strokeLinecap="round" />
      </svg>
      <p className="text-sm text-neutral-400">Sonuç bulunamadı</p>
      <p className="mt-1 text-xs text-neutral-600">Farklı bir anahtar kelime dene ya da filtreleri temizle</p>
    </div>
  );
}
