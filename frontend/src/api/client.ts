import type { SearchParams, SearchResponse } from "../types";

const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

// search, backend'in /search endpoint'ini çağırır. Provider'lar henüz
// bağlanmadığı için (bkz. proje notları) bu çağrı şu an boş bir dizi
// dönebilir — bu HATA değil, beklenen bir ara durumdur. UI bunu
// "sonuç bulunamadı" durumuyla aynı şekilde ele alır.
export async function search(params: SearchParams): Promise<SearchResponse> {
  const url = new URL("/search", API_URL);
  url.searchParams.set("q", params.q);
  if (params.type !== "all") {
    url.searchParams.set("type", params.type);
  }
  url.searchParams.set("page", String(params.page));
  url.searchParams.set("page_size", String(params.pageSize));

  const res = await fetch(url.toString());
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error ?? `İstek başarısız: ${res.status}`);
  }
  return res.json();
}

// triggerRefresh, /refresh endpoint'ini tetikler — provider'lardan
// yeniden veri çekilmesini sağlar. Dashboard'da bir "Yenile" butonu
// olarak kullanılabilir.
export async function triggerRefresh(): Promise<{ status: string }> {
  const url = new URL("/refresh", API_URL);
  const res = await fetch(url.toString(), { method: "POST" });
  if (!res.ok) {
    throw new Error(`Yenileme başarısız: ${res.status}`);
  }
  return res.json();
}
