# Lokal Kurulum ve Çalıştırma

Bu doküman, projeyi kendi makinende (Docker Compose ile) ayağa kaldırmanın güncel ve
doğrulanmış yolunu anlatır. Mimari kararlar için `architecture_tr.md`, paket yapısı için
`package-structure_tr.md`'ye bakın.

## Ön koşullar

| Araç | Ne için | Kontrol komutu |
|---|---|---|
| Docker + Docker Compose | Tüm sistemi (Mongo, ES, Prometheus, Grafana, Loki, mock provider, frontend...) ayağa kaldırmak için | `docker --version` / `docker compose version` |
| Go (1.24+) | Sadece backend'i Docker olmadan lokal çalıştırmak/derlemek istersen | `go version` |
| Node.js (18+) + npm | Sadece dashboard'u Docker olmadan hızlı iterasyonla geliştirmek istersen | `node --version` |

Günlük kullanım için Docker yeterli — Go ve Node sadece o katmanı ayrı geliştirirken lazım.

## Hızlı başlangıç (Docker Compose — önerilen)

```bash
cp .env.example .env      # gerekirse provider URL'lerini kendi değerlerinle değiştir
go mod tidy                # go.sum dosyasını üretir — Dockerfile bunu bekliyor, ŞART
docker compose up -d --build
```

Başlangıç sırası bilinçli olarak katmanlı: önce MongoDB + Elasticsearch healthcheck'lerini
geçer, ancak ondan sonra `app` başlar; `app` healthy olduktan sonra Prometheus/Loki/
Promtail/Grafana/Alertmanager başlar (detay için `docker-compose.yml`'deki yorumlara bak).

Servisler ayakta olduğunda:

| Servis | Adres |
|---|---|
| API | http://localhost:8080 |
| MongoDB | mongodb://localhost:27017 |
| Mongo Express (Mongo verisini tarayıcıdan görüntüleme) | http://localhost:8081 (admin/admin) |
| Elasticsearch | http://localhost:9200 |
| Elasticvue (ES verisini tarayıcıdan görüntüleme) | http://localhost:8082 |
| Prometheus | http://localhost:9090 |
| Alertmanager | http://localhost:9093 |
| Grafana | http://localhost:3000 (admin/admin) |
| Loki | http://localhost:3100 (Grafana → Explore üzerinden kullanılır) |
| json-provider-a (kendi mock'umuz) | http://localhost:4001/json-a |
| Dashboard (frontend) | http://localhost:5173 |

**Doğrulama:**
```bash
docker compose ps                 # hepsi "healthy" ya da "running" görünmeli
docker compose logs app --tail=50 # "sunucu başlatılıyor" satırını görmelisin
curl "http://localhost:8080/search?q=go&page=1&page_size=5"
```

## Kod değiştirdikten sonra yeniden build etme

Sadece `app` servisini etkileyen bir değişiklik yaptıysan:

```bash
docker compose build --no-cache app
docker compose up -d
```

**`--no-cache` kullanmayı unutma** — Docker bazen "hiçbir şey değişmedi" sanıp eski binary'i
içeren bir layer'ı tekrar kullanabiliyor; özellikle sık dosya değişikliği yapılan bir geliştirme
akışında bu, container'ın gerçekte GÜNCEL kodu çalıştırmadığı halde `docker compose ps`'te
normal görünmesine yol açabilir.

Sadece belirli servisleri build/restart etmek istersen, servis adını belirt:
```bash
docker compose build --no-cache prometheus alertmanager
docker compose up -d prometheus alertmanager
```

## Testleri çalıştırma

Proje kök dizininde:
```bash
go test ./... -v
```
- `./...` tüm alt paketlerdeki testleri bulur (`internal/search`, `internal/ingestion`,
  `internal/provider`).
- `-v` her testin PASS/FAIL durumunu tek tek gösterir.

Beklenen: üç paket de `ok` dönmeli, `FAIL` görmemelisin. GoLand kullanıyorsan aynı testleri
fonksiyonun solundaki ▶ ikonuna tıklayarak ya da `internal` klasörüne sağ tıklayıp
"Run 'go test ./...'" diyerek de çalıştırabilirsin.

## Manuel outage/resilience testi

`docker-compose.yml`'de otomatik zamanlanmış bir kesinti simülasyonu YOK — provider
kesintileri elle tetikleniyor, böylece istediğin anda, öngörülebilir şekilde test edebiliyorsun:

```bash
docker compose stop json-provider-a     # provider'ı durdur
docker compose logs app -f              # circuit breaker'ın closed→open→half-open geçişini izle
docker compose start json-provider-a    # provider'ı geri kaldır
```

Beklenen akış: birkaç ardışık hatadan sonra (~5-10sn) breaker **Open**'a geçer; provider
tekrar ayağa kalktıktan sonra ~10-20 saniye içinde **HalfOpen** üzerinden **Closed**'a döner.
Aynı anda `http://localhost:9090/alerts`'te `ProviderCircuitBreakerOpen` kuralının
Firing→Resolved geçişini, `http://localhost:9093`'te de Alertmanager'a düşen alarmı
görebilirsin.

Elasticsearch'i durdurup Mongo↔ES tutarlılık mekanizmasını (indexed flag + reconcile) test
etmek istersen aynı mantıkla `docker compose stop elasticsearch` / `start elasticsearch`
kullanabilirsin — `content_pending_indexing` metriğinin ES kapalıyken arttığını,
tekrar açıldığında sıfıra düştüğünü gözlemleyebilirsin.

## Dashboard'u Docker olmadan geliştirmek (hızlı iterasyon)

Docker'daki `frontend` servisi her değişiklikte yeniden build gerektirir — günlük
geliştirmede Vite'ın kendi dev server'ını kullanmak çok daha hızlıdır:

```bash
cd frontend
npm install
cp .env.example .env      # VITE_API_URL zaten localhost:8080'i gösteriyor
npm run dev
```

`http://localhost:5173` adresinde canlı yeniden yükleme (hot reload) ile çalışır. Backend'in
(`go run ./cmd/api` ya da Docker Compose'daki `app` servisi) ayrıca ayakta olması gerekir.

## Sık karşılaşılan sorunlar

- **`app` container'ı `unhealthy` görünüyor / beklenmeyen loglar basıyor**: `cmd/api/main.go`
  dosyasının içeriğini kontrol et — dosya adları çakıştığı için (`mocks/json-provider-a/main.go`
  ile `cmd/api/main.go` ikisi de `main.go`) kopyalarken yanlış içerik yanlış path'e
  yazılabiliyor. `cmd/api/main.go`'nun ilk satırında `// cmd/api, uygulamanın tek giriş
  noktasıdır...` yorumu olmalı; `// mockjsonprovider, ...` yazıyorsa dosyalar karışmış demektir.
- **Kod değiştirdim ama container hâlâ eski davranışı gösteriyor**: `docker compose build
  --no-cache <servis>` ile temiz rebuild yap (yukarıdaki bölüm).
- **cAdvisor ile container bazlı CPU/RAM metriği yok**: bilerek — Docker Desktop for Mac'in
  LinuxKit VM'inde mount propagation kısıtı nedeniyle güvenilir çalışmadığı için tamamen
  kaldırıldı. Sadece Go runtime metrikleri (heap, goroutine, CPU) tutuluyor, bkz.
  `architecture_tr.md` bölüm 10.
- **`go.mod`'daki Go sürümü makinendekinden farklı**: `go.mod`'da `go 1.27` yazıyor; kendi
  makinende daha güncel bir Go kuruluysa sorun çıkarmaz, daha eskiyse Go'yu güncelle.