export const metadata = {
  title: "My Addresses",
};

export default function AddressesPage() {
  return (
    <div className="space-y-2">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
        Addresses
      </h1>
      <p className="text-sm text-[color:var(--ink-900)] opacity-50">
        Your saved addresses will appear here.
      </p>
    </div>
  );
}
