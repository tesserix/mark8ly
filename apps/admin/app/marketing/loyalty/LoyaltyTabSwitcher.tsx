"use client";

import { useState, type ReactNode } from "react";

interface LoyaltyTabSwitcherProps {
  programTab: ReactNode;
  membersTab: ReactNode;
  referralsTab: ReactNode;
}

const tabs = [
  { key: "program", label: "Program" },
  { key: "members", label: "Members" },
  { key: "referrals", label: "Referrals" },
] as const;

type TabKey = (typeof tabs)[number]["key"];

export function LoyaltyTabSwitcher({
  programTab,
  membersTab,
  referralsTab,
}: LoyaltyTabSwitcherProps) {
  const [activeTab, setActiveTab] = useState<TabKey>("program");

  return (
    <div className="space-y-6">
      {/* Tab bar */}
      <nav
        className="flex gap-0 border-b border-[color:var(--ink-900)]/6"
        role="tablist"
      >
        {tabs.map((tab) => (
          <button
            key={tab.key}
            role="tab"
            aria-selected={activeTab === tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2.5 text-sm font-medium transition-colors ${
              activeTab === tab.key
                ? "border-b-2 border-[color:var(--ink-900)] text-[color:var(--ink-900)]"
                : "text-[color:var(--ink-900)]/40 hover:text-[color:var(--ink-900)]/70"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {/* Tab content */}
      <div>
        {activeTab === "program" && programTab}
        {activeTab === "members" && membersTab}
        {activeTab === "referrals" && referralsTab}
      </div>
    </div>
  );
}
