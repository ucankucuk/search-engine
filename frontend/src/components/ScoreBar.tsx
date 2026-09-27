interface ScoreBarProps {
  label: string;
  value: number;
  // max: bu değere göre bar genişliği oranlanır. Yeni puanlama formülü
  // (views/1000, reactions/reading_time*5 gibi) SINIRSIZ değerler
  // üretebiliyor — sabit bir 0-1 varsayımı artık geçersiz. Bunun yerine
  // görüntülenen sonuç kümesindeki en yüksek değeri "dolu bar" kabul
  // ediyoruz; böylece bar, o sayfadaki içerikler arasında ANLAMLI bir
  // görsel kıyaslama sağlıyor.
  max: number;
}

export function ScoreBar({ label, value, max }: ScoreBarProps) {
  const pct = max > 0 ? Math.round(Math.max(0, Math.min(1, value / max)) * 100) : 0;
  const colorClass = pct < 40 ? "bg-score-low" : pct < 70 ? "bg-score-mid" : "bg-score-high";

  return (
    <div className="flex items-center gap-2 text-xs text-neutral-500">
      <span className="w-20 shrink-0">{label}</span>
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-white/10">
        <div className={`h-full rounded-full ${colorClass} transition-all duration-500`} style={{ width: `${pct}%` }} />
      </div>
      <span className="w-12 shrink-0 text-right tabular-nums text-neutral-400">{value.toFixed(1)}</span>
    </div>
  );
}
