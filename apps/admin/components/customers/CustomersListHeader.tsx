// components/customers/CustomersListHeader.tsx
//
// Page header for /customers. Matches the same header rhythm as
// `AdminPage` so list and settings pages read consistently.

export function CustomersListHeader() {
  return (
    <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
      <div className="min-w-0 flex-1 space-y-3">
        <p className="eyebrow">Relationships</p>
        <h1
          id="customers-heading"
          className="font-serif text-4xl font-medium tracking-tight text-foreground text-balance sm:text-5xl"
        >
          Customers
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          Everyone who has shopped at your store. View order history, manage
          tags, and handle account status.
        </p>
      </div>
    </header>
  );
}
