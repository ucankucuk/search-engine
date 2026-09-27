export function LoadingSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="animate-pulse rounded-2xl border border-white/10 bg-white/[0.03] p-4">
          <div className="h-4 w-3/4 rounded bg-white/10" />
          <div className="mt-2 h-3 w-1/3 rounded bg-white/5" />
          <div className="mt-4 space-y-2">
            <div className="h-1.5 w-full rounded-full bg-white/5" />
            <div className="h-1.5 w-full rounded-full bg-white/5" />
            <div className="h-1.5 w-full rounded-full bg-white/5" />
          </div>
        </div>
      ))}
    </div>
  );
}
