-- star-top-28-leads.sql
-- Marks the 28 highest-priority warm-up targets (from Mark8ly's
-- triage of the no-website Instagram seller scrape) as is_starred=true
-- in tesserix_admin.leads. Idempotent — running twice flips no extra
-- rows. Source: ~/Downloads/insta-scrape/warmup_top30.csv (after
-- AU/IN scoring). The list is small enough that a single IN(...) is
-- cleaner than a temp table.
--
-- Run against the prod tesserix-postgres cluster:
--   kubectl -n tesserix run star-seed-$(date +%s) --rm -i --restart=Never \
--     --image=postgres:16-alpine \
--     --labels="app.kubernetes.io/component=database-init,app.kubernetes.io/part-of=tesserix" \
--     --overrides='{"metadata":{"annotations":{"sidecar.istio.io/inject":"false"}}}' \
--     --env "PGPASSWORD=$DB_PASS" --env "PGSSLMODE=require" \
--     --command -- psql -h tesserix-postgres-rw.tesserix.svc.cluster.local \
--     -U tesserix_admin -d tesserix_admin -v ON_ERROR_STOP=on \
--     < docs/marketing/seeds/star-top-28-leads.sql

\set ON_ERROR_STOP on

BEGIN;

-- Sanity: confirm at least one of the listed handles exists in the
-- leads table before flipping anything. If the CSV diverged from the
-- imported set, abort instead of silently doing nothing.
DO $$
DECLARE
  matched int;
BEGIN
  SELECT count(*) INTO matched
    FROM leads
   WHERE instagram_handle = ANY (ARRAY[
      'olieveandolie',  -- #1  score=13  Australia
      'decor_diva18',  -- #2  score=13  India
      'belove.handmadecrafts',  -- #3  score=13  India
      'white_musk_crafts',  -- #4  score=12  India
      'jays_silks_and_sarees',  -- #5  score=12  India
      'shilpini__',  -- #6  score=12  India
      'kissmyhideaustralia',  -- #7  score=11  Australia
      'themysticalhippieau',  -- #8  score=11  Australia
      'a__k__fashion_jewellery_house',  -- #9  score=11  India
      'kamakshi__clothing',  -- #10  score=10  India
      'shree_krishna.art',  -- #11  score=10  India
      'yards_of_elegance_',  -- #12  score=10  India
      'ethnic__bazaar',  -- #13  score=10  India
      'crochetartbykalpana',  -- #14  score=10  India
      'paints_and_strokes',  -- #15  score=10  India
      'designsby_ky',  -- #16  score=7  Australia
      'advital',  -- #17  score=5  Australia
      'thread.and.sparkle',  -- #18  score=5  Australia
      '_foreveretched',  -- #19  score=3  Australia
      'a_little_bit_of_heaven_1',  -- #20  score=3  Australia
      'miqiling999',  -- #21  score=3  Australia
      'cloud_wovenbyjp',  -- #22  score=2  Australia
      'zatti.au',  -- #23  score=1  Australia
      'bpcustomsaus',  -- #24  score=0  Australia
      'frecklesanddust',  -- #25  score=0  Australia
      '__shot__oclock',  -- #26  score=-1  Australia
      'littlelinesbybec',  -- #27  score=-1  Australia
      'jazzymadebanners'  -- #28  score=-1  Australia
    ]);
  IF matched = 0 THEN
    RAISE EXCEPTION 'No matching handles found — did the CSV drift from the imported set?';
  END IF;
  RAISE NOTICE 'star-seed: matched % of % handles, flipping is_starred=true', matched, 28;
END $$;

UPDATE leads
   SET is_starred = true,
       updated_at = NOW()
 WHERE instagram_handle = ANY (ARRAY[
     'olieveandolie',
     'decor_diva18',
     'belove.handmadecrafts',
     'white_musk_crafts',
     'jays_silks_and_sarees',
     'shilpini__',
     'kissmyhideaustralia',
     'themysticalhippieau',
     'a__k__fashion_jewellery_house',
     'kamakshi__clothing',
     'shree_krishna.art',
     'yards_of_elegance_',
     'ethnic__bazaar',
     'crochetartbykalpana',
     'paints_and_strokes',
     'designsby_ky',
     'advital',
     'thread.and.sparkle',
     '_foreveretched',
     'a_little_bit_of_heaven_1',
     'miqiling999',
     'cloud_wovenbyjp',
     'zatti.au',
     'bpcustomsaus',
     'frecklesanddust',
     '__shot__oclock',
     'littlelinesbybec',
     'jazzymadebanners'
   ])
   AND is_starred = false;  -- only flip rows that aren't already starred

COMMIT;

-- After run, verify on the leads admin page (Starred-only filter)
-- or via SQL:
--   SELECT count(*) FROM leads WHERE is_starred = true;
