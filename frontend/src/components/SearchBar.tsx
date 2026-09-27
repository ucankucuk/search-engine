interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
}

// Marka renkleriyle: ikincil mavi (#0087FF) ana kenarlık/parlama, mor
// (#9966ff) "akıllı arama" aksanı (sparkle), gönder butonu birincil→ikincil
// (yeşil→mavi) geçişli — pill şeklinde, koyu zemin üzerinde, hero'nun
// büyümüş başlığıyla orantılı, daha büyük bir boyutta.
export function SearchBar({ value, onChange }: SearchBarProps) {
  return (
    <div className="relative mx-auto max-w-3xl">
      <div className="group flex items-center gap-3 rounded-full border border-brand-secondary/30 bg-white/5 px-6 py-4 shadow-glow-sm backdrop-blur-sm transition-shadow focus-within:border-brand-secondary/60 focus-within:shadow-glow">
        <svg
          width="22"
          height="22"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          className="shrink-0 text-neutral-400"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="m21 21-4.3-4.3" strokeLinecap="round" />
        </svg>

        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Aramaya başlamak için bir anahtar kelime yaz"
          className="w-full bg-transparent text-base text-neutral-100 placeholder-neutral-500 outline-none sm:text-lg"
        />

        <span className="shrink-0 text-brand-purple">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2l1.6 5.6L19 9l-5.4 1.6L12 16l-1.6-5.4L5 9l5.4-1.4L12 2z" />
          </svg>
        </span>

        <button
          type="button"
          aria-label="Ara"
          style={{ backgroundImage: "linear-gradient(135deg, #2DC44D, #0087FF)" }}
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-white transition-transform hover:scale-105"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
            <path d="M2 21l21-9L2 3v7l15 2-15 2z" />
          </svg>
        </button>
      </div>
    </div>
  );
}
