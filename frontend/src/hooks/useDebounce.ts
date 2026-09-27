import { useEffect, useState } from "react";

// Kullanıcı yazarken her tuşta API'ye istek atmamak için — 350ms
// gecikmeyle son değeri döner. Arama kutusunun "her harfte network
// isteği atmaması" gerektiğini söylemiştik, bunun karşılığı burası.
export function useDebounce<T>(value: T, delayMs = 350): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);

  return debounced;
}
