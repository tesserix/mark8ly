-- 000143_product_description_plain_text.up.sql
--
-- Re-sanitization for SanitizerPolicyVersion 2 (spec §14.14): product
-- descriptions become plain text. v1 stored bluemonday HTML output, which
-- every reader renders as text, so buyers saw "&amp;" for "&".
--
-- v1 output escapes every literal "<" as "&lt;", so any "<" still present
-- is a tag. Block-level closers and <br> become newlines before tags are
-- stripped, so paragraph breaks survive; a trailing one is trimmed.
-- Entities decode with &amp; last, so a merchant's literal "&lt;"
-- (stored "&amp;lt;") comes back as "&lt;".
-- "&amp;" chains collapse to one "&": re-saving the escaped text from the
-- edit form compounded it one level per save.
UPDATE products
   SET description = rtrim(
       regexp_replace(
         replace(replace(replace(replace(replace(replace(
           regexp_replace(
             regexp_replace(
               regexp_replace(description, '<br\s*/?>', E'\n', 'gi'),
               '</(p|li|h[2-4]|blockquote)>', E'\n', 'gi'),
             '<[^>]*>', '', 'g'),
           '&lt;', '<'), '&gt;', '>'), '&quot;', '"'), '&#34;', '"'), '&#39;', ''''), '&#x27;', ''''),
         '&amp;(amp;)*', '&', 'g'),
       E'\n')
 WHERE description ~ '[<&]';
