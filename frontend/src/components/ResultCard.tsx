import { motion } from "framer-motion";
import type { ScoredContent } from "../types";
import { TypeBadge } from "./TypeBadge";
import { ScoreBar } from "./ScoreBar";

interface ResultCardProps {
  content: ScoredContent;
  maxBase: number;
  maxEngagement: number;
}

export function ResultCard({ content, maxBase, maxEngagement }: ResultCardProps) {
  const publishedDate = new Date(content.published_at).toLocaleDateString("tr-TR");

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.25 }}
      className="rounded-2xl border border-white/10 bg-white/[0.03] p-4 backdrop-blur-sm transition-all hover:border-brand-secondary/30 hover:shadow-glow-sm"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-medium text-neutral-100">{content.title}</h3>
          <div className="mt-1 flex items-center gap-2 text-xs text-neutral-500">
            <span>{content.provider}</span>
            <span>·</span>
            <span>{publishedDate}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span
            className="rounded-full border border-brand-primary/30 bg-brand-primary/10 px-2 py-0.5 text-xs font-semibold tabular-nums text-brand-primary"
            title="Final Skor = Temel Puan × Tür Katsayısı + Güncellik + Etkileşim"
          >
            {content.final_score.toFixed(1)}
          </span>
          <TypeBadge type={content.type} />
        </div>
      </div>

      <div className="mt-3 space-y-1.5">
        <ScoreBar label="Temel Puan" value={content.base_score} max={maxBase} />
        <ScoreBar label="Etkileşim" value={content.engagement_score} max={maxEngagement} />
        <ScoreBar label="Güncellik" value={content.freshness_score} max={5} />
      </div>
    </motion.div>
  );
}
