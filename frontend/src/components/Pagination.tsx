interface PaginationProps {
page: number;
totalPages: number;
total: number;
pageSize: number;
onPageChange: (page: number) => void;
onPageSizeChange: (pageSize: number) => void;
}

const PAGE_SIZE_OPTIONS = [5, 10, 25, 50];

function getPageNumbers(current: number, total: number): (number | "ellipsis")[] {
const delta = 1;
const pages: (number | "ellipsis")[] = [1];

const left = Math.max(2, current - delta);
const right = Math.min(total - 1, current + delta);

if (left > 2) pages.push("ellipsis");
for (let i = left; i <= right; i++) pages.push(i);
if (right < total - 1) pages.push("ellipsis");
if (total > 1) pages.push(total);

return pages;
}

export function Pagination({ page, totalPages, total, pageSize, onPageChange, onPageSizeChange }: PaginationProps) {
if (total === 0) return null;

const pages = getPageNumbers(page, totalPages);

return (
<div className="mt-8 flex flex-col items-center gap-4">
<p className="text-sm text-neutral-500">
<span className="font-medium text-neutral-300">{total.toLocaleString("tr-TR")}</span> sonuç
{totalPages > 1 && (
<>
{" "}
· Sayfa <span className="font-medium text-neutral-300">{page}</span> / {totalPages}
</>
)}
</p>

{totalPages > 1 && (
<div className="flex items-center gap-1.5">
<button
onClick={() => onPageChange(Math.max(1, page - 1))}
disabled={page === 1}
className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-neutral-300 transition-colors hover:bg-white/10 disabled:pointer-events-none disabled:opacity-40"
>
Önceki
</button>

<div className="flex items-center gap-1">
{pages.map((p, idx) =>
p === "ellipsis" ? (
<span key={`ellipsis-${idx}`} className="px-1.5 text-sm text-neutral-600" aria-hidden>
…
</span>
) : (
<button
key={p}
onClick={() => onPageChange(p)}
aria-current={p === page ? "page" : undefined}
className={`h-8 min-w-8 rounded-lg px-2 text-sm transition-colors ${
                    p === page
                      ? "bg-brand-secondary/20 text-brand-secondary"
                      : "text-neutral-400 hover:bg-white/10 hover:text-neutral-200"
                  }`}
>
{p}
</button>
),
)}
</div>

<button
onClick={() => onPageChange(Math.min(totalPages, page + 1))}
disabled={page === totalPages}
className="rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-neutral-300 transition-colors hover:bg-white/10 disabled:pointer-events-none disabled:opacity-40"
>
Sonraki
</button>
</div>
)}

<label className="flex items-center gap-2 text-sm text-neutral-500">
Sayfa başına
<select
value={pageSize}
onChange={(e) => onPageSizeChange(Number(e.target.value))}
className="appearance-none rounded-lg border border-white/10 bg-white/5 px-2.5 py-1 text-sm text-neutral-300 outline-none transition-colors hover:bg-white/10 focus:bg-white/10"
>
{PAGE_SIZE_OPTIONS.map((size) => (
<option key={size} value={size}>
{size}
</option>
))}
</select>
</label>
</div>
);
}