# Mimari Tasarım

Bu doküman, `search-engine` projesinin genel mimarisini, önemli tasarım kararlarını ve
bunların gerekçelerini anlatır. Kod içindeki paket bazlı detaylar için
`package-structure_tr.md`, kurulum/çalıştırma için `local-setup_tr.md` dosyalarına bakın.

![Mimari akış diyagramı](architecture-flow.png)

![Mimari genel bakış](architecture-overview.png)

## 1. Genel bakış

Servis üç ana akıştan oluşur:

1. **Ingestion** — üç içerik provider'ından (2 gerçek + 1 kendi yazdığımız mock) periyodik
   olarak veri çeker, normalize eder, MongoDB'ye yazar, Elasticsearch'e indeksler.
2. **Search** — kullanıcı sorgusu Elasticsearch'te adayları bulur, uygulama içindeki özel
   puanlama formülüyle yeniden sıralanır ve sayfalanır.
3. **Observability** — her iki akış da metrik/log üretir; Prometheus toplar, Grafana
   görselleştirir, Alertmanager kritik durumlarda uyarı üretir.

## 2. Yüksek seviye veri akışı

```
provider'lar (JSON x2, XML x1)
        │  Resilient (timeout + retry + circuit breaker)
        ▼
  ingestion.Job.RunAll
        │  UpsertMany                       │ FindUnindexed (Reconcile)
        ▼                                    │
     MongoDB  ◀──────────────────────────────┘
   (source of truth)
        │  IndexMany
        ▼
  Elasticsearch (arama görünümü)
        │  Search (BM25 recall)
        ▼
  search.Service (scoring + paginate)
        │
        ▼
  HTTP API (/search, /refresh, /metrics)
        │
        ▼
  Frontend (React, Vite)

  app:/metrics ──▶ Prometheus ──▶ Alertmanager
                       │
                       ▼
                    Grafana ◀── Loki (Promtail ile toplanan container logları)
```

## 3. Neden MongoDB + Elasticsearch ikilisi (CQRS benzeri ayrım)

- **MongoDB = source of truth.** Her provider'dan gelen veri `external_id + provider`
  bileşimiyle upsert edilir (idempotent). Provider'a özgü, ortak şemaya oturmayan alanlar
  `RawFields` içinde kaybolmadan saklanır.
- **Elasticsearch = türetilmiş, yeniden inşa edilebilir arama görünümü.** Sadece arama ve
  sıralama için gereken alanlar indekslenir; `RawFields` bilerek indekslenmez. ES'in kendisi
  hiçbir zaman otorite değildir — index silinse bile Mongo'dan uçtan uca yeniden inşa
  edilebilir.
- Bu ayrım, CQRS'in "yazma modeli" / "okuma modeli" ayrımının basitleştirilmiş bir örneğidir:
  yazma Mongo'ya, okuma (arama) Elasticsearch'e gider.

## 4. Dayanıklılık (resilience) katmanı

`internal/provider/resilient.go`, her provider'ı bir Decorator ile sarar:

- **Timeout** — her fetch çağrısına üst sınır (`FetchTimeout`, varsayılan 5sn).
- **Retry + exponential backoff** (`sethvargo/go-retry`) — geçici hatalarda 3 deneme.
- **Circuit breaker** (`sony/gobreaker`) — `ConsecutiveFailures > 5` olunca **Open**'a geçer;
  `Timeout=10s` sonra **HalfOpen**'da bir deneme hakkı verir; `MaxRequests=3` ardışık
  başarıyla **Closed**'a döner.

Önemi: bir provider çökerse diğerlerinin ingestion turu etkilenmez, ve çökmüş provider'a
gereksiz yere sürekli istek atılmaz — breaker açıkken istekler ağa hiç gitmeden anında
reddedilir (`"circuit breaker is open"`).

## 5. Veri tutarlılığı: MongoDB → Elasticsearch (basitleştirilmiş outbox)

**Problem**: Mongo'ya yazma ile Elasticsearch'e indeksleme iki ayrı, atomik olmayan işlemdir
("dual write" problemi). Mongo'ya yazma başarılı, ES'e indeksleme başarısız olursa: veri var
ama aranamıyor.

**Uygulanan çözüm** (`domain.Content.Indexed` + `IndexedAt`):

- Her `Content` dokümanı bir `indexed: bool` alanı taşır; yeni yazılan her kayıt
  `indexed=false` ile başlar.
- `ingestion.Job.runOne`, Mongo'ya yazdıktan sonra ES'e indekslemeyi dener; başarılı olursa
  `MarkIndexed` ile `indexed=true` yapar.
- Her ingestion turunun (`RunAll`) sonunda `Reconcile()` çalışır: `indexed=false` kalan (en
  fazla `reconcileBatchSize=1000`) kayıtları bulur, ES'e yeniden indekslemeyi dener.
- Böylece tutarsızlık penceresi kalıcı değildir — **en fazla bir sonraki ingestion turu
  kadar geçicidir**.
- `content_pending_indexing` (Prometheus gauge) bu backlog'u gözlemlenebilir kılar;
  `ContentIndexingBacklog` alert kuralı 2 dakikadır sıfırlanmayan bir backlog'u uyarır.

**Neden tam bir outbox pattern / Kafka değil**: Gerçek bir outbox pattern, content yazımıyla
aynı transaction'da bir "outbox event" koleksiyonuna yazıp bunu ayrı bir worker'ın (ya da
Debezium gibi bir CDC aracının Mongo oplog'unu dinleyip Kafka'ya yayınlamasıyla) tüketmesini
gerektirir. Bu proje ölçeğinde (tek node MongoDB, tek consumer, tek event tipi) bu
over-engineering olurdu (transaction'lar için replica set gerekir, Kafka/Debezium kurulumu ek
operasyonel yük getirir). Bunun yerine aynı fikrin **basitleştirilmiş, tek-koleksiyonlu** hâli
uygulandı: doküman kendisi hem veri hem "bekleyen event" rolünü üstleniyor.

## 6. Arama ve puanlama

- Elasticsearch sadece **recall/filtre** katmanıdır — `candidateWindowSize=500` ile en
  alakalı (BM25) ilk 500 adayı getirir.
- Final sıralama BM25 skoruyla değil, `internal/scoring` paketindeki özel formülle yapılır:
  `Final Skor = Temel Puan × Tür Katsayısı + Güncellik Puanı + Etkileşim Puanı`. Bu
  "retrieve then re-rank" deseni, arama alakalılığını iş kurallarına dayalı
  popülerlik/güncellik sıralamasından ayırır.
- `internal/search/service.go` sonuçları sıralar, sayfalar (`normalizePaging`) ve girdiyi
  doğrular (`validateQuery`). Bilinçli tasarım farkı: geçersiz arama girdisi (boş/çok uzun
  keyword, geçersiz tip) **sessizce düzeltilmez**, `ErrInvalidQuery` ile `400` döner; sayfalama
  parametreleri (`page`, `page_size`) ise **sessizce** güvenli varsayılanlara çekilir, çünkü
  bunlar genelde UI'ın ilk render'ından kaynaklanan zararsız değerlerdir.

## 7. Observability

- **Metrikler** (`internal/observability/metrics`): `provider_circuit_state`,
  `provider_requests_total`, `provider_request_duration_seconds`,
  `search_request_duration_seconds`, `content_pending_indexing`. Prometheus bunları
  `/metrics`'ten 10 saniyede bir toplar.
- **Loglama** (`internal/observability/logger`): `slog` ile JSON formatında structured
  logging; her HTTP isteği bir `request_id` (UUID) ile etiketlenir
  (`middleware/request_id.go`), Loki üzerinden bu ID ile uçtan uca log takibi yapılabilir.
- **Dashboard'lar** (Grafana): Go runtime metrikleri (heap, goroutine, CPU) ve Provider
  Health (circuit breaker durumu, request rate, p95 latency).
- **Alerting** (Prometheus + Alertmanager): 5 kural — `ProviderCircuitBreakerOpen`,
  `HighSearchLatency`, `SearchEngineAPIDown`, `HighProviderFailureRate`,
  `ContentIndexingBacklog`. Prometheus kuralları **değerlendirir**, Alertmanager bildirimi
  **gruplar/yönetir** — bu ayrım bilinçli: şu an gerçek bir bildirim kanalı (Slack/e-posta)
  yok, `alertmanager.yml`'deki `receivers` bloğu bilerek boş (null receiver) bırakıldı,
  production'da değişecek tek yer orası olurdu.

## 8. Dayanıklılık ve operasyonel olgunluk

- **Graceful shutdown** (`cmd/api/main.go`): `SIGINT`/`SIGTERM` yakalanır
  (`signal.NotifyContext`), HTTP sunucusu devam eden istekleri bitirmesi için 15 saniye süre
  alır (`server.Shutdown`), ardından Mongo bağlantısı düzgün kapatılır.
- **Rate limiting** (`internal/transport/http/middleware/ratelimit.go`): IP başına,
  bağımlılıksız (elle yazılmış) token-bucket; sadece `/search` ve `/refresh`'e uygulanır —
  `/metrics`'e bilerek uygulanmaz (Prometheus'un düzenli scrape'i etkilenmesin diye).
- **Input validation**: `search.Service.validateQuery`, açıkça geçersiz girdiyi reddeder
  (bkz. bölüm 6).
- **Testler**: `internal/search`, `internal/ingestion`, `internal/provider` paketleri için
  table-driven unit testler; sahte (fake/stub) implementasyonlarla gerçek Mongo/ES/HTTP
  bağımlılığı olmadan izole test edilir (bkz. `package-structure_tr.md`'deki test dosyaları
  listesi).

## 9. Kullanılan teknolojiler (tech stack)

**Backend (Go 1.27)**

| Kütüphane | Versiyon | Ne için |
|---|---|---|
| `go.mongodb.org/mongo-driver` | v1.17.1 | MongoDB client |
| `github.com/elastic/go-elasticsearch/v8` | v8.15.0 | Elasticsearch client |
| `github.com/sony/gobreaker` | v0.5.0 | Circuit breaker |
| `github.com/sethvargo/go-retry` | v0.3.0 | Retry + exponential backoff |
| `github.com/prometheus/client_golang` | v1.20.5 | Prometheus metrik istemcisi |
| `github.com/google/uuid` | v1.6.0 | `request_id` üretimi |
| `log/slog` (standart kütüphane) | — | Structured (JSON) logging |

**Frontend**

| Teknoloji | Versiyon | Ne için |
|---|---|---|
| React | 18.3 | UI kütüphanesi |
| TypeScript | 5.6 | Tip güvenliği |
| Vite | 5.4 | Build aracı + dev server |
| Tailwind CSS | 3.4 | Utility-first styling |
| Framer Motion | 11 | Animasyonlar (sonuç kartları, geçişler) |
| Nginx | 1.27-alpine | Production'da statik dosya sunumu |

**Altyapı (Docker Compose ile orkestrasyon)**

| Servis | İmaj/Versiyon | Rol |
|---|---|---|
| MongoDB | `mongo:7` | Source of truth |
| Elasticsearch | `docker.elastic.co/elasticsearch/elasticsearch:8.15.0` | Arama indeksi |
| Prometheus | `prom/prometheus:v2.55.0` | Metrik toplama + alert değerlendirme |
| Alertmanager | `prom/alertmanager:v0.27.0` | Alert gruplama/routing |
| Grafana | `grafana/grafana:11.2.0` | Dashboard görselleştirme |
| Loki + Promtail | `grafana/loki:3.1.0` + `grafana/promtail:3.1.0` | Log toplama/sorgulama |
| Mongo Express | `mongo-express:1.0.2` | Mongo verisini tarayıcıdan görüntüleme (geliştirme aracı) |
| Elasticvue | `cars10/elasticvue:1.16.0` | ES verisini tarayıcıdan görüntüleme (geliştirme aracı) |
| Go build image | `golang:1.27-alpine` → `alpine:3.20` | Çok aşamalı (multi-stage) Docker build |
| Node build image | `node:20-alpine` | Frontend build aşaması |

## 10. Bilinçli tasarım kararları (özet)

| Karar | Alternatif | Neden bu seçildi |
|---|---|---|
| `indexed` flag + `Reconcile` | Tam outbox + Kafka/Debezium | Bu ölçekte over-engineering olurdu |
| Elle yazılmış rate limiter | `golang.org/x/time/rate` | Mekanizmayı kod üzerinden şeffaf tutmak için |
| BM25 (recall) + uygulama içi re-rank | Sadece ES sıralaması | Puanlama formülü spesifikasyonun bir parçası, ES'in relevance skoruyla karışmamalı |
| Alertmanager'da null receiver | Gerçek Slack/e-posta entegrasyonu | Kapsam dışı, ama "rule evaluation vs. notification routing" ayrımını göstermek için altyapı kuruldu |
| Docker Desktop'ta cAdvisor | Container bazlı CPU/RAM metrikleri | Docker Desktop'ın LinuxKit VM'inde mount propagation kısıtı nedeniyle güvenilir çalışmadı, kaldırıldı — sadece Go runtime metrikleri tutuldu |