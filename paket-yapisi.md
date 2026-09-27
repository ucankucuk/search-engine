# Paket Yapısı ve Best Practice Kaynağı

Bu yapı, Go ekosisteminde en yaygın referans kabul edilen
[`golang-standards/project-layout`](https://github.com/golang-standards/project-layout) ve güncel
(2026) Go mimari yazılarında tekrarlanan üç temel prensibe dayanıyor:

1. **`cmd/` sadece giriş noktasıdır.** İçinde iş mantığı yazılmaz; sadece `internal/` paketlerini
   çağırıp birbirine bağlar ("composition root").
2. **`internal/` altında katmana değil domain'e/sorumluluğa göre paketleme yapılır.** Yani tüm
   "modeller" tek bir `models/` klasöründe toplanmaz; her paket kendi sorumluluğunu (provider
   erişimi, arama, puanlama, HTTP taşıma...) taşır. `internal/` klasörü Go derleyicisi tarafından
   zorlanan bir kural: içindeki paketler proje dışından asla import edilemez.
3. **Bağımlılıklar içe doğru akar (clean architecture'ın Go'ya uyarlanmış hali).** `domain`
   paketi hiçbir şeyi import etmez; `provider`, `storage`, `search` gibi dış dünyaya dokunan
   paketler `domain`'i import eder ama birbirlerini değil; `cmd/api` en dışta durur ve hepsini
   birbirine bağlar. Bu proje `pkg/` klasörü kullanmıyor çünkü dışarıya (başka bir projeye) açık
   edilecek, kütüphane olarak paylaşılacak bir kod yok — best practice kaynakları da bu durumda
   `pkg/`'ı gereksiz bir "no-op" katman olarak görüyor.

## Klasör ağacı

```
search-engine/
├── go.mod
├── README.md
├── cmd/
│   └── api/
│       └── main.go
└── internal/
    ├── domain/
    │   ├── content.go
    │   └── errors.go
    ├── config/
    │   └── config.go
    ├── provider/
    │   ├── provider.go
    │   ├── fetcher.go
    │   ├── adapter.go
    │   ├── resilient.go
    │   ├── json/
    │   │   ├── fetcher.go
    │   │   └── normalizer.go
    │   └── xml/
    │       ├── fetcher.go
    │       └── normalizer.go
    ├── ingestion/
    │   ├── job.go
    │   └── scheduler.go
    ├── storage/
    │   ├── mongo/
    │   │   └── repository.go
    │   └── elastic/
    │       └── index.go
    ├── scoring/
    │   └── scoring.go
    ├── search/
    │   └── service.go
    ├── transport/
    │   └── http/
    │       ├── router.go
    │       ├── handler/
    │       │   ├── search.go
    │       │   └── refresh.go
    │       └── middleware/
    │           └── request_id.go
    └── observability/
        ├── logger/
        │   └── logger.go
        └── metrics/
            └── metrics.go
```

## Her paketin amacı

### `cmd/api`
**main.go** — uygulamanın tek giriş noktası (composition root). Config'i okur, Mongo/Elasticsearch
bağlantılarını açar, provider'ları kaydeder, resilience katmanını uygular, servisleri ve HTTP
handler'larını kurar, sunucuyu başlatır. **İçinde hiçbir iş kuralı yoktur** — sadece "hangi
parça hangi parçaya bağlanıyor" bilgisi.

### `internal/domain`
Uygulamanın çekirdek veri modelleri (`Content`, `ScoredContent`, `SearchQuery`) ve paylaşılan
hata tipleri (`ErrContentNotFound` vb.) burada yaşar. **Hiçbir şeyi import etmez** — bu paket
clean architecture'daki "entities" katmanının karşılığı. Diğer tüm paketler bunu import
edebilir; bu paket hiçbirini import etmez. Provider'ın JSON mı XML mi olduğunu, verinin Mongo'da
mı Elasticsearch'te mi durduğunu bilmez.

### `internal/config`
Ortam değişkenlerini okuyup tip-güvenli bir `Config` struct'ına çevirir. Projede başka hiçbir
paket doğrudan `os.Getenv` çağırmaz — "hangi env değişkenleri var" sorusunun tek cevabı burasıdır.

### `internal/provider`
Dış içerik sağlayıcılarına erişimi `Provider` interface'i arkasında soyutlar.
- `provider.go`: `Provider` interface'i + self-registering registry (`Register`, `All`, `Get`).
- `fetcher.go`: `RawFetcher` (bağlantı) ve `Normalizer` (format çevirme) interface'leri —
  "nasıl bağlanılır" ile "nasıl standarda çevrilir" bilerek ayrı tutulur.
- `adapter.go`: `RawFetcher` + `Normalizer`'ı birleştirip tam bir `Provider` üreten `Adapter`.
- `resilient.go`: herhangi bir `Provider`'ı timeout + retry/backoff + circuit breaker ile saran
  `Resilient` decorator'ı.

### `internal/provider/json`, `internal/provider/xml`
Her provider ailesinin somut implementasyonu (gerçek HTTP çağrısı + gerçek parse mantığı).
Bu paketler **sadece `cmd/api/main.go` tarafından import edilir** — `ingestion` paketi bunların
varlığından habersizdir, sadece `provider.All()` üzerinden konuşur. Yeni bir provider eklerken
değişmesi gereken yer burasıdır (yeni bir alt paket + `main.go`'da tek satır).

### `internal/ingestion`
- `job.go`: kayıtlı tüm provider'ları (ya da tek birini) çalıştırıp sonucu storage katmanına
  yazan `Job`. Kendi dar interface'lerini (`ContentWriter`, `Indexer`) tanımlar — somut Mongo/ES
  implementasyonlarını değil, bu interface'leri bilir (dependency inversion).
- `scheduler.go`: `Job`'ı periyodik olarak tetikleyen zamanlayıcı (`time.Ticker` tabanlı; ihtiyaç
  halinde `robfig/cron` ile değiştirilebilir).

### `internal/storage/mongo`
MongoDB'ye erişimi kapsülleyen `Repository`. "System of record" katmanı — `external_id` bazlı
upsert ile idempotency garanti edilir. `ingestion.ContentWriter` interface'ini karşılar ama bunu
implicit olarak yapar (Go'da `implements` anahtar kelimesi yok).

### `internal/storage/elastic`
Elasticsearch'e erişimi kapsülleyen `Client`. Arama-optimize edilmiş, Mongo'dan yeniden inşa
edilebilir görünüm katmanı. Hem `ingestion.Indexer` (yazma) hem `search.Searcher` (okuma)
interface'lerini karşılar.

### `internal/scoring`
Provider'dan gelen ham popülerlik değerlerini standart puana çeviren, içerik türüne göre
ağırlıklandırma yapan saf fonksiyonlar. HTTP'yi, veritabanını, hiçbir dış bağımlılığı bilmez —
bu yüzden birim testi en kolay olan pakettir.

### `internal/search`
"Kullanıcı arama yaptığında ne olur" akışının iş mantığı: Elasticsearch'ten aday çeker,
`scoring` ile yeniden puanlar, sıralar, sayfalar. HTTP'nin request/response nesnelerini hiç
görmez — bu sayede aynı mantık ileride bir CLI'dan ya da gRPC'den de çağrılabilir.

### `internal/transport/http`
- `router.go`: hangi path'in hangi handler'a, hangi middleware zincirinden geçerek ulaştığını
  tanımlayan "wiring" katmanı.
- `handler/`: HTTP isteğini domain tiplerine çevirip iş mantığına devreden ince katman
  (`search.go`, `refresh.go`). Handler'lar hiçbir iş kuralı içermez.
- `middleware/`: tüm endpoint'lere uygulanan çapraz-kesit davranışlar — `request_id.go` her
  isteğe bir UUID atayıp context üzerinden taşır (Loki'de `request_id` ile log arama bunun
  üzerine kurulu) ve istek loglarını yazar.

### `internal/observability/logger`
Structured logger'ın (JSON, stdout) tek kurulum noktası. İleride `zap`'e geçiş gerekirse
değişmesi gereken tek dosya budur.

### `internal/observability/metrics`
Prometheus'a expose edilen tüm custom metriklerin (`provider_circuit_state`,
`provider_requests_total`, `provider_request_duration_seconds`, `search_request_duration_seconds`)
tek tanım noktası. Provider ya da search paketi kendi metriğini dağınık şekilde tanımlamaz,
buradan import eder.

## Bu yapı neden bu şekilde kuruldu

- **Test edilebilirlik**: `scoring` ve `search` gibi saf iş mantığı paketleri hiçbir dış
  bağımlılık import etmediği için mock'lamaya gerek kalmadan birim testi yazılabilir.
- **Değişim izolasyonu**: Mongo'dan başka bir veritabanına geçilecek olsa, sadece
  `internal/storage/mongo` değişir; `ingestion`, `search`, `scoring` hiç etkilenmez (çünkü onlar
  interface'lerle konuşuyor, somut tiplerle değil).
- **Genişletilebilirlik**: yeni bir provider eklemek `internal/provider/<yeni>/` altında iki
  dosya + `main.go`'da bir satır demek; mevcut hiçbir dosya değişmez.
- **Okunabilirlik**: bir mülakatçı ya da yeni katılan bir geliştirici, klasör isimlerine bakarak
  "arama mantığı nerede", "resilience nerede" sorularının cevabını saniyeler içinde bulabilir —
  bu da "temiz ve anlaşılır kod yapısı" beklentisini doğrudan karşılıyor.
