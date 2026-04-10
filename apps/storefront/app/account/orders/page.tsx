export const metadata = {
  title: "My Orders",
};

export default function OrdersPage() {
  return (
    <div className="space-y-2">
      <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--ink-900)]">
        Orders
      </h1>
      <p className="text-sm text-[color:var(--ink-900)] opacity-50">
        Your order history will appear here.
      </p>
    </div>
  );
}
