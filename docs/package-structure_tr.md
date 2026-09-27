# Paket Yapısı ve Best Practice Kaynağı

Bu yapı [`golang-standards/project-layout`](https://github.com/golang-standards/project-layout)
— Go ekosisteminde en çok referans verilen community layout'u — ve 2026 itibarıyla Go mimarisi
yazınında tekrar eden üç prensibi takip eder:

1. **`cmd/` sadece giriş noktasıdır.** Orada iş mantığı yaşamaz; sadece `internal/`
   paketlerini birbirine bağlar ("composition root").
2. **`internal/` içindeki paketler türe göre değil, domain/sorumluluğa göre organize edilir.**
   Modeller ortak bir `models/` klasörüne atılmaz; her paket kendi sorumluluğunu taşır
   (provider erişimi, arama, puanlama, HTTP transport...). `internal/` zaten Go derleyicisi
   tarafından uygulanan bir kuraldır: içindeki paketler projenin dışından asla import
   edilemez.
3. **Bağımlılıklar içe doğru akar** (Go'nun clean architecture yorumu). `domain` paketi
   hiçbir şey import etmez; `provider`, `storage`, `search` gibi dışa dönük paketler
   `domain`'i import eder ama birbirlerini etmez; `cmd/api` en dışta durur ve her şeyi
   birbirine bağlar. Bu projede `pkg/` klasörü yok çünkü başka bir projeyle paylaşılacak bir
   kütüphane kodu yok — best-practice kaynakları bu durumda `pkg/`'ı genelde gereksiz bir
   no-op katman olarak görür.

## Klasör ağacı

```
search-engine/
├── go.mod, go.sum
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── README.md                           # EN
├── README_tr.md                        # TR
├── docs/
│   ├── architecture_tr.md / architecture_en.md
│   ├── package-structure_tr.md / package-structure_en.md
│   ├── local-setup_tr.md / local-setup_en.md
│   ├── architecture-flow.png
│   └── architecture-overview.png
├── cmd/
│   └── api/
│       └── main.go                    # composition root + graceful shutdown
├── internal/
│   ├── domain/
│   │   ├── content.go                 # Content, ScoredContent, SearchQuery/Result, Indexed flag
│   │   └── errors.go
│   ├── config/
│   │   └── config.go                  # tüm env değişkenleri tek noktada
│   ├── provider/
│   │   ├── provider.go                # Provider arayüzü + registry
│   │   ├── fetcher.go                 # RawFetcher / Normalizer arayüzleri
│   │   ├── adapter.go                 # Adapter (RawFetcher+Normalizer birleşimi)
│   │   ├── resilient.go               # timeout + retry + circuit breaker decorator
│   │   ├── resilient_test.go
│   │   ├── json/
│   │   │   ├── fetcher.go
│   │   │   └── normalizer.go
│   │   └── xml/
│   │       ├── fetcher.go
│   │       └── normalizer.go
│   ├── ingestion/
│   │   ├── job.go                     # RunAll / RunOne / Reconcile
│   │   ├── job_test.go
│   │   └── scheduler.go               # periyodik tetikleme (time.Ticker)
│   ├── storage/
│   │   ├── mongo/
│   │   │   └── repository.go          # UpsertMany, FindUnindexed, MarkIndexed
│   │   └── elastic/
│   │       └── index.go               # IndexMany, Search, index mapping
│   ├── scoring/
│   │   └── scoring.go                 # saf puanlama formülü
│   ├── search/
│   │   ├── service.go                 # arama iş mantığı + validasyon + sayfalama
│   │   └── service_test.go
│   ├── transport/
│   │   └── http/
│   │       ├── router.go              # path → handler → middleware zinciri
│   │       ├── handler/
│   │       │   ├── search.go
│   │       │   └── refresh.go
│   │       └── middleware/
│   │           ├── request_id.go      # UUID + structured request logging
│   │           ├── cors.go            # tek origin'e izin veren CORS
│   │           └── ratelimit.go       # IP başına token-bucket rate limiter
│   └── observability/
│       ├── logger/
│       │   └── logger.go              # slog kurulumu (JSON, stdout)
│       └── metrics/
│           └── metrics.go             # tüm Prometheus metriklerinin tanım noktası
├── mocks/
│   └── json-provider-a/               # kendi yazdığımız 3. (JSON) mock provider
│       ├── main.go
│       ├── go.mod                     # ayrı bir Go modülü — ana projeden bağımsız derlenir
│       └── Dockerfile
├── frontend/                          # React + TypeScript + Vite + Tailwind + Framer Motion
│   └── src/
│       ├── App.tsx, main.tsx, index.css, types.ts
│       ├── api/client.ts              # backend'e HTTP istekleri
│       ├── components/                # SearchBar, FilterBar, ResultList, ResultCard,
│       │                              # ScoreBar, TypeBadge, Pagination, EmptyState, ...
│       └── hooks/useDebounce.ts
└── deployments/
    ├── prometheus/
    │   ├── prometheus.yml             # scrape config + rule_files + alerting.alertmanagers
    │   └── rules.yml                  # 5 alert kuralı
    ├── alertmanager/
    │   └── alertmanager.yml           # route + null receiver
    ├── grafana/provisioning/
    │   ├── datasources/datasources.yml
    │   └── dashboards/
    │       ├── dashboards.yml
    │       ├── provider-health.json
    │       └── system-health.json
    ├── loki/loki-config.yml
    └── promtail/promtail-config.yml   # container loglarını Loki'ye taşır
```

## Her paketin amacı

### `cmd/api`
**main.go** — uygulamanın tek giriş noktası (composition root). Config'i okur, Mongo/ES
bağlantılarını açar, provider'ları kaydeder, resilience katmanını uygular, servisleri ve HTTP
handler'larını birbirine bağlar, sunucuyu başlatır. **Graceful shutdown** burada yönetilir:
`SIGINT`/`SIGTERM` yakalanır, HTTP sunucusu 15 saniye içinde devam eden istekleri bitirir,
Mongo bağlantısı düzgün kapatılır. **Hiçbir iş kuralı içermez** — sadece hangi parçanın hangi
parçaya bağlandığı bilgisi.

### `internal/domain`
Uygulamanın çekirdek veri modellerini (`Content`, `ScoredContent`, `SearchQuery`,
`SearchResult`) ve ortak hata tiplerini (`ErrContentNotFound`, `ErrInvalidQuery`) taşır.
**Hiçbir şey import etmez** — clean architecture'daki "entities" katmanının Go karşılığıdır.
`Content` struct'ı ayrıca `Indexed`/`IndexedAt` alanlarını taşır — Mongo↔ES tutarlılık
mekanizmasının (basitleştirilmiş outbox, bkz. `architecture_tr.md` bölüm 5) durumunu tutan,
API yanıtına hiç sızmayan (`json:"-"`) bir iç bookkeeping alanıdır.

### `internal/config`
Ortam değişkenlerini tip-güvenli bir `Config` struct'ına okur. Projede başka hiçbir paket
doğrudan `os.Getenv` çağırmaz. `RateLimitRPS`/`RateLimitBurst` alanları rate limiter'ı
yapılandırır.

### `internal/provider`
Dış içerik provider'larına erişimi bir `Provider` arayüzü arkasında soyutlar.
- `provider.go`: `Provider` arayüzü + kendi kendine kayıt olan registry (`Register`, `All`,
  `Get`).
- `fetcher.go`: `RawFetcher` (bağlantı) ve `Normalizer` (format dönüşümü) arayüzleri —
  "nasıl bağlanılır" bilinçli olarak "nasıl normalize edilir"den ayrılmıştır.
- `adapter.go`: bir `RawFetcher` ile bir `Normalizer`'ı birleştirip tam bir `Provider`
  üreten `Adapter`.
- `resilient.go`: herhangi bir `Provider`'ı timeout + retry/backoff + circuit breaker ile
  saran `Resilient` decorator'ı.

### `internal/provider/json`, `internal/provider/xml`
Her provider ailesinin somut implementasyonu (gerçek HTTP çağrısı + gerçek parse mantığı).
Bu paketler **sadece `cmd/api/main.go` tarafından import edilir** — `ingestion` paketinin
bunların varlığından haberi yoktur, sadece `provider.All()` ile konuşur. Yeni bir provider
eklendiğinde değişen yer burasıdır (yeni bir alt paket + `main.go`'da tek satır).

### `internal/ingestion`
- `job.go`: `Job` — kayıtlı tüm provider'ları çalıştırıp sonucu storage katmanına yazar.
  Kendi dar arayüzlerini (`ContentWriter`, `Indexer`) tanımlar — somut Mongo/ES
  implementasyonlarını değil, bu arayüzleri bilir (dependency inversion). `RunAll`'ın
  sonunda `Reconcile()` çalışır: `indexed=false` kalan kayıtları bulup ES'e yeniden
  indekslemeyi dener (bkz. `architecture_tr.md` bölüm 5).
- `scheduler.go`: `Job`'ı periyodik tetikleyen zamanlayıcı (`time.Ticker` tabanlı).

### `internal/storage/mongo`
`Repository` — MongoDB erişimini kapsüller. Sistem-of-record katmanıdır; idempotency
`external_id + provider` üzerinden upsert ile garanti edilir. `ingestion.ContentWriter`
arayüzünü örtük olarak karşılar (Go'da `implements` anahtar kelimesi yoktur).
`FindUnindexed`/`MarkIndexed`, basitleştirilmiş outbox deseninin okuma/yazma tarafıdır.

### `internal/storage/elastic`
`Client` — Elasticsearch erişimini kapsüller. Aramaya özel, Mongo'dan yeniden inşa
edilebilir görünüm katmanı. Hem `ingestion.Indexer`'ı (yazma) hem `search.Searcher`'ı
(okuma) karşılar. `candidateWindowSize=500` ile BM25'e göre en alakalı ilk 500 adayı getirir
— final sıralama `scoring` paketine bırakılır.

### `internal/scoring`
Provider'ın ham popülerlik değerlerini içerik türüne göre ağırlıklandırılmış standart bir
skora çeviren saf fonksiyonlar. HTTP, veritabanı ya da başka hiçbir dış bağımlılık bilmez —
bu yüzden en kolay unit test edilebilen pakettir.

### `internal/search`
"Kullanıcı arama yaptığında ne olur" iş mantığı: Elasticsearch'ten adayları çeker, `scoring`
ile yeniden puanlar, sıralar, sayfalar, girdiyi doğrular (`validateQuery`). HTTP
request/response nesnelerini hiç görmez — aynı mantık ileride bir CLI ya da gRPC servisinden
de çağrılabilir.

### `internal/transport/http`
- `router.go`: hangi path'in hangi handler'a, hangi middleware zincirinden geçerek
  ulaştığını tanımlayan wiring katmanı. Rate limiter'ı kurar, `/search` ve `/refresh`'e
  uygular (`/metrics`'e uygulamaz).
- `handler/`: HTTP isteğini domain tiplerine çevirip iş mantığına devreden ince katman
  (`search.go`, `refresh.go`). Handler'lar iş kuralı içermez.
- `middleware/`: her endpoint'e uygulanan çapraz kesen davranışlar —
  `request_id.go` her isteğe bir UUID atar ve context üzerinden taşır (Loki'de
  `request_id` ile log aramayı mümkün kılan şey budur), `cors.go` tek bir origin'e
  (dashboard) izin verir, `ratelimit.go` IP başına token-bucket rate limiting uygular.

### `internal/observability/logger`
Structured logger'ın (JSON, stdout, `slog`) tek kurulum noktası. İleride `zap`'e geçiş
gerekirse değişecek tek dosya budur.

### `internal/observability/metrics`
Prometheus'a açılan her özel metriğin tek tanım noktası (`provider_circuit_state`,
`provider_requests_total`, `provider_request_duration_seconds`,
`search_request_duration_seconds`, `content_pending_indexing`). Ne provider ne de search
paketi kendi metriğini ayrıca tanımlar — hepsi buradan import eder.

### `mocks/json-provider-a`
Gerçek JSON/XML provider'ların şema ve kavramlarının bir karışımını,
çok daha büyük bir veri setiyle (varsayılan 20.000 kayıt) sunan, kendi yazdığımız üçüncü
provider. Ana projeden **ayrı bir Go modülü** (`go.mod`) ve ayrı bir Docker image'ı —
`internal/provider/json`'daki Normalizer bu API'yi gerçek provider'la aynı Normalizer'la
parse edebilir çünkü şema kasıtlı olarak birebir aynı tutulmuştur.

### `frontend`
React 18 + TypeScript + Vite ile yazılmış, Tailwind CSS ile stillendirilmiş ve Framer Motion
ile animasyonlu arama arayüzü. `api/client.ts` backend'e istek atar, `components/` arama
kutusu, filtre, sonuç listesi/kartı, skor çubuğu, sayfalama gibi UI parçalarını taşır.
Docker'da (`node:20-alpine` ile build edilip `nginx:1.27-alpine` ile) statik olarak servis
edilir; geliştirme sırasında `npm run dev` ile Vite'ın kendi sunucusu kullanılır (bkz.
`local-setup_tr.md`).

### `deployments`
Go kodunun dışında kalan tüm altyapı konfigürasyonu: Prometheus scrape/alert kuralları,
Alertmanager route'u, Grafana'nın otomatik provision edilen veri kaynakları ve
dashboard'ları, Loki ve Promtail konfigürasyonu. Bu dosyalar `docker-compose.yml` içinden
ilgili container'lara salt-okunur olarak mount edilir.

## Bu yapı neden bu şekilde kuruldu

- **Test edilebilirlik**: `scoring` ve `search` gibi saf iş mantığı paketleri hiçbir dış
  bağımlılık import etmez, bu yüzden hiçbir şey mock'lamadan unit test edilebilir — nitekim
  `internal/search`, `internal/ingestion`, `internal/provider` paketlerinin tamamı fake/stub
  implementasyonlarla test edilmiştir (`*_test.go` dosyaları).
- **Değişimin izolasyonu**: Mongo'dan başka bir veritabanına geçmek sadece
  `internal/storage/mongo`'yu değiştirir; `ingestion`, `search`, `scoring` etkilenmez, çünkü
  bunlar somut tiplerle değil arayüzlerle konuşur.
- **Genişletilebilirlik**: yeni bir provider eklemek `internal/provider/<yeni>/` altında iki
  dosya + `main.go`'da tek satır demektir — mevcut hiçbir dosya değişmez.
- **Okunabilirlik**: yeni bir takım üyesi klasör isimlerine bakarak
  "arama mantığı nerede," "resilience katmanı nerede" sorularının cevabını saniyeler içinde
  bulabilir.
- **Operasyonel netlik**: Go kodu (`internal/`, `cmd/`) ile altyapı konfigürasyonu
  (`deployments/`) ve dış process'ler (`mocks/`, `frontend/`) net bir şekilde ayrılmıştır —
  hangi değişikliğin hangi container'ın rebuild edilmesini gerektirdiği klasör yapısından
  belli olur.