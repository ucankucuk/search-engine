import { AnimatePresence, motion } from "framer-motion";
import type { ScoredContent } from "../types";
import { ResultCard } from "./ResultCard";
import { EmptyState } from "./EmptyState";
import { LoadingSkeleton } from "./LoadingSkeleton";

interface ResultListProps {
  results: ScoredContent[];
  loading: boolean;
  error: string | null;
}

export function ResultList({ results, loading, error }: ResultListProps) {
  if (error) {
    return (
      <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-300">
        {error}
      </div>
    );
  }

  if (loading) {
    return <LoadingSkeleton />;
  }

  if (results.length === 0) {
    return <EmptyState />;
  }

  // base_score ve engagement_score formülü sınırsız değer üretebiliyor
  // (bkz. ScoreBar.tsx) — bu sayfadaki en yüksek değeri "dolu bar" kabul
  // ederek her kartın bar'ını bu sonuç kümesine göre orantılıyoruz.
  const maxBase = Math.max(1, ...results.map((r) => r.base_score));
  const maxEngagement = Math.max(1, ...results.map((r) => r.engagement_score));

  return (
    <motion.div layout className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <AnimatePresence mode="popLayout">
        {results.map((item) => (
          <ResultCard
            key={`${item.provider}-${item.external_id}`}
            content={item}
            maxBase={maxBase}
            maxEngagement={maxEngagement}
          />
        ))}
      </AnimatePresence>
    </motion.div>
  );
}
