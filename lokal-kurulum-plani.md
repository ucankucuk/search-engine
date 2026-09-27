# Lokal Kurulum ve Çalıştırma Planı

Bu doküman, şimdiye kadar tasarladığımız arama motoru servisini sıfırdan kendi bilgisayarında
ayağa kaldırman için izleyeceğin adımları sırasıyla anlatır. Her adımın sonunda "bunu nasıl
doğrularım" notu var — bir adımda takılırsan bir sonrakine geçmeden orada dur.

## Ön koşul: kurulu olması gerekenler

| Araç | Ne için | Kontrol komutu |
|---|---|---|
| Go (1.23+) | Backend'i derlemek/çalıştırmak için | `go version` |
| Docker + Docker Compose | Mongo, Elasticsearch, Prometheus, Grafana, Loki'yi ayağa kaldırmak için | `docker --version` / `docker compose version` |
| Node.js (18+) + npm | Dashboard (React) için | `node --version` |
| Git | Versiyon kontrolü için (opsiyonel ama önerilir) | `git --version` |
| Bir HTTP test aracı | Endpoint'leri denemek için | `curl --version` ya da Postman/Insomnia kurulu olsun |

Eksik olan varsa önce onu kur, sonra devam et.

---

## Adım 1 — Proje iskeletini yerleştir

1. Sana gönderilen `search-engine.zip` dosyasını aç, bir çalışma klasörüne yerleştir (ör.
   `~/projects/search-engine`).
2. Terminalde o klasöre gir: `cd ~/projects/search-engine`
3. `go mod tidy` çalıştır — bu, `go.mod`'da tanımlı bağımlılıkları (gobreaker, go-retry,
   prometheus client, mongo-driver, uuid) indirir ve `go.sum` dosyasını oluşturur.
4. `go build ./...` çalıştır — hiçbir dosya derleme hatası vermemeli. Verirse (import
   path'lerinde küçük bir uyumsuzluk çıkma ihtimali var, ben bu ortamda derleyip test
   edemedim) hatayı bana yapıştır, birlikte düzeltelim.

**Doğrulama:** `go build ./...` sessizce (hatasız) biterse bu adım tamam.

---

## Adım 2 — Mock provider'ları hazırla

Elinde hazır bir JSON ve bir XML mock API olduğunu söylemiştin; üçüncü (JSON) provider'ı da sen
yazacaktın. Bu üçünü de projeden bağımsız, ayrı ayrı ayakta tutman gerekiyor çünkü Go uygulaması
bunlara HTTP ile istek atacak.

1. Eğer mock'ların hazır bir Docker image'ı ya da basit bir Express/Flask sunucusu şeklindeyse,
   onları da bir `docker-compose.yml` içine servis olarak ekleyebiliriz (Adım 4'te buna değinilecek).
2. Eğer mock'lar sende lokal olarak `npm start` ya da `python app.py` gibi komutlarla ayağa
   kalkıyorsa, bunları ayrı terminal sekmelerinde çalışır halde tut.
3. Her mock'un URL'sini not al (ör. `http://localhost:4001/json-a`,
   `http://localhost:4002/json-b`, `http://localhost:4003/xml`) — Adım 5'te `.env` dosyasına
   bunları yazacaksın.

**Doğrulama:** `curl http://localhost:4001/json-a` gibi bir komutla her üç mock'tan da anlamlı
bir JSON/XML çıktısı aldığından emin ol.

---

## Adım 3 — `internal/provider/json` ve `xml` paketlerindeki Normalizer'ları kendi mock verine göre uyarla

Şu an `internal/provider/json/normalizer.go` ve `internal/provider/xml/normalizer.go`
dosyalarındaki `payload`/`feed` struct'ları **örnek/varsayımsal alan adları** içeriyor (`id`,
`title`, `kind`, `views`, `published_at` gibi). Senin gerçek mock API'lerinin döndüğü gerçek
alan adlarıyla bunları eşleştirmen gerekiyor.

1. Her mock'un döndüğü örnek bir yanıtı `curl` ile çek, alan adlarını not al.
2. `internal/provider/json/normalizer.go` içindeki `payload` struct'ının JSON tag'lerini
   (`json:"..."`) gerçek alan adlarına göre güncelle.
3. Aynısını `internal/provider/xml/normalizer.go` içindeki `feed`/`entry` struct'ları ve
   `xml:"..."` tag'leri için yap.
4. Üçüncü JSON provider'ını (kendi yazacağın) da `internal/provider/json/fetcher.go` içindeki
   `Register()` fonksiyonuna üçüncü bir `provider.NewAdapter(...)` çağrısı olarak ekle (README'de
   "yeni provider ekleme" adımları var).

**Doğrulama:** Bu adımda henüz çalıştırma yok, sadece kod güncelleniyor — `go build ./...`
tekrar hatasız derlenmeli.

---

## Adım 4 — Docker Compose ile altyapıyı ayağa kaldır (Mongo, Elasticsearch, Prometheus, Grafana, Loki)

Bu projede henüz `docker-compose.yml` dosyasını birlikte yazmadık — bir sonraki adımımız bu
olabilir. Compose dosyası hazır olduğunda buradaki adım şu şekilde işleyecek:

1. Proje kök dizininde `docker-compose up -d` çalıştır.
2. Şu servislerin ayakta olduğunu kontrol et:
   - MongoDB → `docker ps` çıktısında görünmeli, `mongodb://localhost:27017`
   - Elasticsearch → `curl http://localhost:9200` boş bir cluster bilgisi dönmeli
   - Prometheus → `http://localhost:9090` tarayıcıda açılmalı
   - Grafana → `http://localhost:3000` tarayıcıda açılmalı (varsayılan giriş genelde admin/admin)
   - Loki → Grafana'dan "Explore" sekmesinde veri kaynağı olarak görünmeli

**Doğrulama:** `docker ps` çıktısında beş servis de (mongo, elasticsearch, prometheus, grafana,
loki) "Up" durumunda görünmeli.

*(İstersen bu adımı beklemeden şimdi Compose dosyasını birlikte yazabiliriz — sıradaki mesajında
söylemen yeterli.)*

---

## Adım 5 — `.env` dosyasını oluştur

1. Proje kök dizininde bir `.env` dosyası oluştur (ya da ortam değişkenlerini terminalde
   `export` ile ver).
2. `internal/config/config.go` içindeki değişkenlere karşılık gelen değerleri gir:
   ```
   HTTP_PORT=8080
   MONGO_URI=mongodb://localhost:27017
   MONGO_DATABASE=search_engine
   ELASTIC_URL=http://localhost:9200
   PROVIDER_A_URL=http://localhost:4001/json-a
   PROVIDER_B_URL=http://localhost:4002/json-b
   PROVIDER_XML_URL=http://localhost:4003/xml
   ```
3. Go'nun `.env` dosyasını otomatik okuması için `godotenv` gibi bir kütüphane eklenebilir ya da
   `main.go`'yu her çalıştırmadan önce terminalde `export $(cat .env | xargs)` ile ortam
   değişkenlerini yükleyebilirsin.

**Doğrulama:** `echo $PROVIDER_A_URL` (export ettiysen) doğru URL'yi göstermeli.

---

## Adım 6 — Go uygulamasını çalıştır

1. `go run ./cmd/api` komutunu çalıştır.
2. Terminalde şu logları görmelisin (structured JSON formatında):
   - `"sunucu başlatılıyor" port=8080`
   - Provider'lar için circuit breaker durumu logları (ilk başta `closed` olmalı)
3. Uygulama ayaktayken başka bir terminalde:
   ```
   curl http://localhost:8080/metrics
   ```
   Prometheus formatında bir metrik listesi dönmeli.

**Doğrulama:** Terminal açık kalıp hata vermeden "sunucu başlatılıyor" mesajını gösteriyorsa bu
adım tamam. Hemen kapanıyorsa (Mongo/ES bağlantı hatası olabilir), hata mesajını oku — muhtemelen
Adım 4'teki servislerden biri ayakta değildir.

---

## Adım 7 — İlk veri çekimini tetikle ve doğrula

1. Uygulama ayaktayken:
   ```
   curl -X POST http://localhost:8080/refresh
   ```
2. Terminal loglarında her provider için `"provider fetch tamamlandı"` satırlarını görmelisin
   (`item_count` alanıyla birlikte).
3. MongoDB'ye bağlanıp (ör. MongoDB Compass ya da `mongosh` ile) `search_engine.contents`
   koleksiyonunda verinin gerçekten yazıldığını kontrol et:
   ```
   mongosh mongodb://localhost:27017/search_engine --eval "db.contents.countDocuments()"
   ```

**Doğrulama:** Koleksiyondaki doküman sayısı 0'dan büyük olmalı ve provider sayına göre mantıklı
görünmeli.

---

## Adım 8 — Arama endpoint'ini dene

1. ```
   curl "http://localhost:8080/search?q=test&page=1&page_size=10"
   ```
2. Şu an `internal/storage/elastic/index.go` içindeki `Search` fonksiyonu **henüz iskelet
   halinde** (boş dönüyor) — bu adımda gerçek bir sonuç almayacaksın. Elasticsearch entegrasyonu
   tamamlanana kadar bu endpoint boş bir liste dönecek, bu beklenen bir durum.
3. Elasticsearch client'ının gerçek implementasyonunu (bulk indexleme + arama sorgusu) birlikte
   yazmamız gereken bir sonraki adım.

**Doğrulama:** İstek 500 hatası vermeden (boş da olsa) bir JSON listesi dönüyorsa altyapı
doğru bağlanmış demektir.

---

## Adım 9 — Grafana dashboard'unu kur

1. `http://localhost:3000` adresine git, Prometheus ve Loki'yi veri kaynağı (data source) olarak
   ekle (Compose dosyası bunları otomatik provision edecek şekilde de yazılabilir).
2. Prometheus için resmi "Go Processes" dashboard'unu import et (dashboard ID: 6671).
3. Kendi "Provider Health" panelini oluştur:
   - `provider_circuit_state` metriğini stat panel olarak ekle, value mapping ile
     0=yeşil/1=sarı/2=kırmızı renklendir.
   - `rate(provider_requests_total[5m])` ile request rate grafiği.
   - `histogram_quantile(0.95, provider_request_duration_seconds_bucket)` ile p95 latency.

**Doğrulama:** Bir provider'ı manuel olarak durdurup (mock sunucusunu kapatıp) `/refresh`
tetiklediğinde, birkaç deneme sonrası Grafana panelinde ilgili provider'ın durumunun
"open" (kırmızı) olduğunu görmelisin — bu senin canlı demo sahnenin provası.

---

## Adım 10 — Dashboard'u (React) ayağa kaldır

Frontend kodunu henüz birlikte yazmadık — bu da bir sonraki adımlarımızdan biri olacak. Hazır
olduğunda:

1. `frontend/` klasörüne gir, `npm install`.
2. `.env` içinde API adresini tanımla: `VITE_API_URL=http://localhost:8080`.
3. `npm run dev` ile geliştirme sunucusunu başlat, tarayıcıda aç.

**Doğrulama:** Dashboard'da arama kutusuna bir kelime yazdığında `/search` isteğinin ağ
sekmesinde (DevTools → Network) gittiğini gör.

---

## Sırada ne var (henüz birlikte yazmadıklarımız)

Bu planı uygularken şu parçalara ihtiyaç duyacaksın ama henüz kodunu yazmadık — istediğin an
sırayla üretebiliriz:

1. **Docker Compose dosyasının tamamı** (Mongo, Elasticsearch, Prometheus, Grafana, Loki,
   Promtail, mock provider'lar, Go API, frontend — hepsi tek `docker-compose.yml`'de)
2. **Elasticsearch client'ının gerçek implementasyonu** (`internal/storage/elastic/index.go`
   içindeki `IndexMany` ve `Search` fonksiyonları şu an boş)
3. **React dashboard'un kodu**
4. **Birim testleri** (`scoring`, `search` paketleri için)
5. **Prometheus scrape config ve Grafana provisioning dosyaları**

Hangisiyle devam etmek istersin?
