"use client";

import { useState, useRef, useEffect } from "react";
import Link from "next/link";
import { Search } from "lucide-react";

interface HelpArticleItem {
  slug: string;
  title: string;
  category: string;
  excerpt: string;
}

interface HelpSearchProps {
  articles: HelpArticleItem[];
}

export function HelpSearch({ articles }: HelpSearchProps) {
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  function handleChange(value: string) {
    setQuery(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => setDebouncedQuery(value), 300);
  }

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  const filtered =
    debouncedQuery.length >= 2
      ? articles.filter((a) =>
          a.title.toLowerCase().includes(debouncedQuery.toLowerCase()),
        )
      : [];

  const showResults = debouncedQuery.length >= 2;

  return (
    <div className="relative w-full sm:w-auto sm:max-w-lg">
      <Search
        className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-foreground-tertiary"
        aria-hidden="true"
      />
      <input
        type="search"
        value={query}
        onChange={(e) => handleChange(e.target.value)}
        placeholder="Search help articles..."
        className="h-12 w-full rounded-md border border-border bg-background pl-10 pr-4 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
        aria-label="Search help articles"
        role="combobox"
        aria-expanded={showResults}
        aria-controls="help-search-results"
        aria-autocomplete="list"
      />

      {showResults && (
        <div
          id="help-search-results"
          className="absolute left-0 right-0 top-full z-10 mt-1 overflow-hidden rounded-md border border-border-subtle bg-[color:var(--background-elevated)] shadow-md"
        >
          {filtered.length === 0 ? (
            <p className="px-4 py-3 text-sm text-foreground-secondary">
              No articles found for &ldquo;{debouncedQuery}&rdquo;
            </p>
          ) : (
            <ul role="listbox" id="help-search-listbox">
              {filtered.map((article) => (
                <li key={article.slug}>
                  <Link
                    href={`/support/help/${article.slug}`}
                    className="block min-h-[44px] px-4 py-3 transition-colors hover:bg-paper-100"
                    role="option"
                    aria-selected={false}
                  >
                    <p className="text-sm font-medium text-foreground">
                      {article.title}
                    </p>
                    <p className="mt-0.5 text-xs text-foreground-secondary">
                      {article.category}
                    </p>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
