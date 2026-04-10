export function CustomersListHeader() {
  return (
    <header className="flex flex-col gap-2">
      <h1
        id="customers-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl text-[color:var(--ink-900)]"
      >
        Customers
      </h1>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Everyone who has shopped at your store. View order history, manage tags,
        and handle account status.
      </p>
    </header>
  );
}
