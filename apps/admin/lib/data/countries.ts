/**
 * Static list of ~50 primary markets for the arbitrage appeal jurisdiction
 * selector. USD/EUR zone + Asia-Pacific + known tenant geos.
 *
 * Sorted alphabetically by label. Each entry is { value: CC, label: name }.
 */
export interface CountryOption {
  value: string
  label: string
}

export const COUNTRY_OPTIONS: CountryOption[] = [
  { value: 'AU', label: 'Australia (AU)' },
  { value: 'AT', label: 'Austria (AT)' },
  { value: 'BD', label: 'Bangladesh (BD)' },
  { value: 'BE', label: 'Belgium (BE)' },
  { value: 'BR', label: 'Brazil (BR)' },
  { value: 'CA', label: 'Canada (CA)' },
  { value: 'CL', label: 'Chile (CL)' },
  { value: 'CN', label: 'China (CN)' },
  { value: 'CO', label: 'Colombia (CO)' },
  { value: 'HR', label: 'Croatia (HR)' },
  { value: 'CY', label: 'Cyprus (CY)' },
  { value: 'CZ', label: 'Czech Republic (CZ)' },
  { value: 'DK', label: 'Denmark (DK)' },
  { value: 'EG', label: 'Egypt (EG)' },
  { value: 'FI', label: 'Finland (FI)' },
  { value: 'FR', label: 'France (FR)' },
  { value: 'DE', label: 'Germany (DE)' },
  { value: 'GH', label: 'Ghana (GH)' },
  { value: 'GR', label: 'Greece (GR)' },
  { value: 'HK', label: 'Hong Kong (HK)' },
  { value: 'HU', label: 'Hungary (HU)' },
  { value: 'IN', label: 'India (IN)' },
  { value: 'ID', label: 'Indonesia (ID)' },
  { value: 'IE', label: 'Ireland (IE)' },
  { value: 'IL', label: 'Israel (IL)' },
  { value: 'IT', label: 'Italy (IT)' },
  { value: 'JP', label: 'Japan (JP)' },
  { value: 'KE', label: 'Kenya (KE)' },
  { value: 'MY', label: 'Malaysia (MY)' },
  { value: 'MX', label: 'Mexico (MX)' },
  { value: 'NL', label: 'Netherlands (NL)' },
  { value: 'NZ', label: 'New Zealand (NZ)' },
  { value: 'NG', label: 'Nigeria (NG)' },
  { value: 'NO', label: 'Norway (NO)' },
  { value: 'PK', label: 'Pakistan (PK)' },
  { value: 'PH', label: 'Philippines (PH)' },
  { value: 'PL', label: 'Poland (PL)' },
  { value: 'PT', label: 'Portugal (PT)' },
  { value: 'RO', label: 'Romania (RO)' },
  { value: 'SA', label: 'Saudi Arabia (SA)' },
  { value: 'SG', label: 'Singapore (SG)' },
  { value: 'ZA', label: 'South Africa (ZA)' },
  { value: 'KR', label: 'South Korea (KR)' },
  { value: 'ES', label: 'Spain (ES)' },
  { value: 'SE', label: 'Sweden (SE)' },
  { value: 'CH', label: 'Switzerland (CH)' },
  { value: 'TW', label: 'Taiwan (TW)' },
  { value: 'TH', label: 'Thailand (TH)' },
  { value: 'TR', label: 'Turkey (TR)' },
  { value: 'AE', label: 'United Arab Emirates (AE)' },
  { value: 'GB', label: 'United Kingdom (GB)' },
  { value: 'US', label: 'United States (US)' },
  { value: 'VN', label: 'Vietnam (VN)' },
]
