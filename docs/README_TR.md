# search-engine

*[English](README.md) | Türkçe*

Diğer tüm dokümanlar [`docs/`](docs/) klasörü altında: detaylı mimari kararlar ve gerekçeleri
için `docs/architecture_tr.md` (TR) / `docs/architecture_en.md` (EN) dosyalarına bakın. Paket
yapısı ve her paketin sorumluluğu için `docs/package-structure_tr.md` (TR) /
`docs/package-structure_en.md` (EN) dosyalarına bakın. Kurulum ve sık karşılaşılan sorunlar
için `docs/local-setup_tr.md` (TR) / `docs/local-setup_en.md` (EN) dosyalarına bakın.


![Mimari akış diyagramı](docs/architecture-flow.png)

![Mimari genel bakış](docs/architecture-overview.png)

## Çalıştırma (lokal, Go ile — geliştirme sırasında)

```
go mod tidy
go run ./cmd/api
```

Ortam değişkenleri (`internal/config/config.go` içinde varsayılanlarıyla tanımlı) için
`.env.example` dosyasını `.env` olarak kopyalayıp kendi değerlerini gir.

## Çalıştırma (Docker Compose ile — tüm sistem)

```
cp .env.example .env   # kendi mock provider URL'lerini gir
go mod tidy             # go.sum dosyasını üretmek için ŞART — Dockerfile bunu bekliyor
docker compose up -d --build
```

Başlangıç sırası bilinçli olarak katmanlı: önce MongoDB + Elasticsearch ayağa kalkıp
healthcheck'lerini geçer, ancak ondan sonra `app` başlar; `app` da healthy olduktan sonra
Prometheus/Loki/Promtail/Grafana başlar (detay için `docker-compose.yml` içindeki yorumlara bak).

Servisler ayakta olduğunda:

| Servis | Adres |
|---|---|
| API | http://localhost:8080 |
| MongoDB | mongodb://localhost:27017 |
| Elasticsearch | http://localhost:9200 |
| Prometheus | http://localhost:9090 |
| Alertmanager | http://localhost:9093 |
| Grafana | http://localhost:3000 (admin/admin) |
| Loki | http://localhost:3100 (Grafana üzerinden Explore ile kullanılır) |
| Dashboard (frontend) | http://localhost:5173 |

Durumu izlemek için: `docker compose ps` — hepsi "healthy" ya da "running" görünmeli.
Loglara bakmak için: `docker compose logs -f app`.
Her şeyi durdurup verileri de silmek için: `docker compose down -v`.

## Dashboard'u ayrı geliştirmek (Docker olmadan, hızlı iterasyon için)

Docker'daki `frontend` servisi her değişiklikte yeniden build gerektirir — günlük geliştirmede
Vite'ın kendi dev server'ını kullanmak çok daha hızlıdır:

```
cd frontend
npm install
cp .env.example .env   # VITE_API_URL zaten localhost:8080'i gösteriyor
npm run dev
```

`http://localhost:5173` adresinde canlı yeniden yükleme (hot reload) ile çalışır. Backend'in
(`go run ./cmd/api` ya da Docker Compose'daki `app` servisi) ayrıca ayakta olması gerekir.