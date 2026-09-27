import { useEffect, useMemo, useRef, useState } from "react";
import { search, triggerRefresh } from "./api/client";
import { useDebounce } from "./hooks/useDebounce";
import { SearchBar } from "./components/SearchBar";
import { FilterBar } from "./components/FilterBar";
import { ResultList } from "./components/ResultList";
import { Pagination } from "./components/Pagination";
import type { ContentType, ScoredContent, SortField } from "./types";

const DEFAULT_PAGE_SIZE = 10;

export default function App() {
  const [query, setQuery] = useState("");
  const [type, setType] = useState<ContentType | "all">("all");
  const [sort, setSort] = useState<SortField>("final_score");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);

  const [results, setResults] = useState<ScoredContent[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const debouncedQuery = useDebounce(query);

  // Arama metni değiştiğinde (type/pageSize değişikliklerinde olduğu
  // gibi) sayfayı 1'e döndürüyoruz — aksi halde kullanıcı 5. sayfadayken
  // yeni, daha dar bir sonuç kümesi döndüren bir arama yaparsa "Sayfa 5 /
  // 2" gibi anlamsız bir duruma düşerdi. type/pageSize değişiklikleri
  // kendi handler'larında (aşağıda) senkron olarak sıfırlanıyor; query
  // debounce'lı olduğu için burada, gerçek arama isteğinden ÖNCE ayrı
  // bir effect'te ele alınıyor.
  const prevQueryRef = useRef(debouncedQuery);
  useEffect(() => {
    if (prevQueryRef.current !== debouncedQuery) {
      prevQueryRef.current = debouncedQuery;
      setPage(1);
    }
  }, [debouncedQuery]);

  useEffect(() => {
    if (debouncedQuery.trim() === "") {
      setResults([]);
      setTotal(0);
      setTotalPages(0);
      setError(null);
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);

    search({ q: debouncedQuery, type, page, pageSize })
        .then((data) => {
          if (!cancelled) {
            setResults(data.results);
            setTotal(data.total);
            setTotalPages(data.total_pages);
          }
        })
        .catch((err: Error) => {
          if (!cancelled) setError(err.message);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });

    return () => {
      cancelled = true;
    };
  }, [debouncedQuery, type, page, pageSize]);

  const sortedResults = useMemo(
      () => [...results].sort((a, b) => b[sort] - a[sort]),
      [results, sort],
  );

  function handleTypeChange(next: ContentType | "all") {
    setType(next);
    setPage(1);
  }

  function handlePageSizeChange(next: number) {
    setPageSize(next);
    setPage(1);
  }

  async function handleRefresh() {
    setRefreshing(true);
    try {
      await triggerRefresh();
    } catch {
      // Yenileme hatası kritik değil — bir sonraki arama yine mevcut
      // veriyle çalışır, kullanıcıyı bloke etmiyoruz.
    } finally {
      setRefreshing(false);
    }
  }

  return (
      <div className="min-h-screen bg-[#08080c] text-neutral-100">
        {/* Hero: başlık + parlayan arama kutusu. Arka plandaki radial glow
          sadece bu bölgede — sonuç listesine indikçe düz koyu zemine döner. */}
        <section className="relative overflow-hidden px-4 pb-16 pt-20 sm:pt-28">
          <div
              aria-hidden
              className="pointer-events-none absolute inset-x-0 top-0 h-[520px] bg-[radial-gradient(ellipse_at_top,_rgba(45,196,77,0.16),_transparent_65%)]"
          />
          <div
              aria-hidden
              className="pointer-events-none absolute inset-x-0 top-0 h-[520px] bg-[radial-gradient(ellipse_at_80%_10%,_rgba(0,135,255,0.16),_transparent_55%)]"
          />
          <div
              aria-hidden
              className="pointer-events-none absolute inset-x-0 top-0 h-[420px] bg-[radial-gradient(ellipse_at_top,_rgba(27,47,111,0.35),_transparent_70%)]"
          />
          <div className="relative mx-auto max-w-4xl text-center">
            <h1 className="text-4xl font-bold tracking-tight sm:text-6xl">
              Doğru içeriği{" "}
              {/* Marka geçişi (birincil → ikincil) satır-içi stille uygulanıyor;
                Tailwind'in bg-clip-text yardımcı sınıfına bağlı kalmadan her
                zaman render olmasını garanti ediyor. */}
              <span
                  style={{
                    backgroundImage: "linear-gradient(90deg, #2DC44D, #0087FF)",
                    WebkitBackgroundClip: "text",
                    backgroundClip: "text",
                    color: "transparent",
                    WebkitTextFillColor: "transparent",
                  }}
              >
              anında
            </span>{" "}
              bul
            </h1>
            <p className="mt-4 text-base text-neutral-400 sm:text-lg">
              Farklı provider'lardan toplanan içerikleri ara, filtrele ve alakalılık skoruna göre sırala
            </p>

            <div className="mt-10">
              <SearchBar value={query} onChange={setQuery} />
            </div>
          </div>
        </section>

        {/* Sonuçlar */}
        <section className="mx-auto max-w-4xl px-4 pb-16">
          <div className="mb-5 flex justify-center">
            <FilterBar type={type} onTypeChange={handleTypeChange} sort={sort} onSortChange={setSort} />
          </div>

          {query.trim() === "" ? null : (
              <>
                <ResultList results={sortedResults} loading={loading} error={error} />

                {!loading && !error && (
                    <Pagination
                        page={page}
                        totalPages={totalPages}
                        total={total}
                        pageSize={pageSize}
                        onPageChange={setPage}
                        onPageSizeChange={handlePageSizeChange}
                    />
                )}
              </>
          )}
        </section>

        {/* Provider yenileme: filtrelerden bağımsız, ekranın köşesine
          sabitlenmiş, orta büyüklükte ikon buton. */}
        <button
            onClick={handleRefresh}
            disabled={refreshing}
            title="Provider'ları yenile"
            aria-label="Provider'ları yenile"
            className="fixed bottom-6 right-6 z-20 flex h-12 w-12 items-center justify-center rounded-full border border-brand-secondary/30 bg-white/5 text-brand-secondary shadow-glow-sm backdrop-blur-md transition-all hover:border-brand-primary/50 hover:text-brand-primary hover:shadow-glow disabled:opacity-50"
        >
          <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              className={refreshing ? "animate-spin" : ""}
          >
            <path d="M21 12a9 9 0 1 1-2.64-6.36" strokeLinecap="round" />
            <path d="M21 3v6h-6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
      </div>
  );
}