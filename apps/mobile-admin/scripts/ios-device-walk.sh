#!/bin/bash
# iOS device-pass walk for mark8ly mobile-admin.
# Relaunches the app before every route so a modal route cannot swallow the
# next deep link — the failure that invalidated 15 of the first 20 shots.
set -u
OUT="$1"          # output directory
APP=com.mark8ly.admin
SCHEME=mark8ly-admin
mkdir -p "$OUT"

shot() { # $1 = route, $2 = name
  xcrun simctl terminate booted "$APP" >/dev/null 2>&1
  sleep 1.5
  xcrun simctl launch booted "$APP" >/dev/null 2>&1
  sleep 9
  if [ "$1" != "/" ]; then
    xcrun simctl openurl booted "$SCHEME://$1" >/dev/null 2>&1
    sleep 6
  fi
  xcrun simctl io booted screenshot "$OUT/$2.png" >/dev/null 2>&1
  echo "  $2"
}

ORDER=7893e9d8-ae50-4ce1-a2f6-7add1ed4c77c
PROD=972a8469-1641-4f82-8b9d-2434e465e150
CUST=2aba99af-f503-420a-a946-402619405de3
REVIEW=2db59202-8236-4a7c-8c08-6e34c4b43807
TICKET=b600be94-751e-4f38-9cfe-044ccffa6d2a
COUPON=a1e7050a-f34a-4023-bf7b-c7c377fbcb7e
GIFT=a44fada3-329b-46fb-b854-84b0984f4cae
SEG=2182c2f4-aee8-49d6-a025-9087a488db35

shot "/"                                        01-dashboard
shot "/orders"                                  02-orders
shot "/orders/$ORDER"                           03-order-detail
shot "/products"                                04-products
shot "/products/$PROD"                          05-product-editor
shot "/products/new"                            06-product-create
shot "/customers"                               07-customers
shot "/customers/$CUST"                         08-customer-detail
shot "/customers/reviews"                       09-reviews
shot "/customers/reviews/$REVIEW"               10-review-detail
shot "/more"                                    11-more
shot "/more/account"                            12-account
shot "/more/security"                           13-security
shot "/more/settings/notification-settings"     14-notification-settings
shot "/more/settings/team"                      15-team
shot "/more/settings/tickets"                   16-tickets
shot "/more/settings/tickets/$TICKET"           17-ticket-detail
shot "/more/marketing/coupons"                  18-coupons
shot "/more/marketing/coupons/$COUPON"          19-coupon-detail
shot "/more/marketing/gift-cards"               20-gift-cards
shot "/more/marketing/gift-cards/$GIFT"         21-gift-card-detail
shot "/more/marketing/campaigns"                22-campaigns
shot "/more/marketing/segments"                 23-segments
shot "/more/marketing/segments/$SEG"            24-segment-detail
shot "/more/marketing/loyalty"                  25-loyalty
shot "/more/settings/branding"                  26-branding
shot "/more/settings/audit-logs"                27-audit-logs
shot "/notifications"                           28-notifications
shot "/more/support"                            29-support

echo "--- identical-screen check ---"
python3 - "$OUT" <<'PY'
from PIL import Image, ImageChops
import sys, glob, os
out = sys.argv[1]
fs = sorted(glob.glob(os.path.join(out, '*.png')))
def load(f):
    im = Image.open(f).convert('L')
    return im.crop((0, 140, im.width, im.height)).resize((150, 320))
ims = {f: load(f) for f in fs}
dupes = []
for i in range(1, len(fs)):
    a, b = fs[i-1], fs[i]
    d = ImageChops.difference(ims[a], ims[b])
    v = sum(d.getdata()) / (150 * 320)
    flag = '  <-- IDENTICAL' if v < 0.5 else ''
    if flag:
        dupes.append((os.path.basename(a), os.path.basename(b)))
    print(f'  {os.path.basename(a)} -> {os.path.basename(b)}: {v:7.2f}{flag}')
print()
print(f'FROZEN PAIRS: {len(dupes)}')
for a, b in dupes:
    print('   ', a, '==', b)
PY
