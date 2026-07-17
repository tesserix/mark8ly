import { useCallback, useState } from "react";
import { View, ScrollView, Switch, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useBranding, useUpdateBranding } from "@/lib/hooks/use-branding";
import { BackHeader, Eyebrow, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import type { Branding } from "@repo/mobile-shared/api/types";
import type { BrandingUpdateBody } from "@repo/mobile-shared/api/branding";

export default function BrandingScreen() {
  const { data: branding, isLoading, error } = useBranding();

  return (
    <Screen>
      <BackHeader eyebrow="SETTINGS" title="Branding" />
      {error ? (
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load branding
          </Text>
        </View>
      ) : isLoading || !branding ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (
        <BrandingForm key={branding.id} branding={branding} />
      )}
    </Screen>
  );
}

function BrandingForm({ branding }: { branding: Branding }) {
  const update = useUpdateBranding();
  const dockPad = useDockClearance();

  const [tagline, setTagline] = useState(branding.tagline ?? "");
  const [announcementText, setAnnouncementText] = useState(branding.announcement_text ?? "");
  const [announcementLink, setAnnouncementLink] = useState(branding.announcement_link ?? "");
  const [announcementActive, setAnnouncementActive] = useState(branding.announcement_active);
  const [instagram, setInstagram] = useState(branding.social_instagram ?? "");
  const [twitter, setTwitter] = useState(branding.social_twitter ?? "");
  const [facebook, setFacebook] = useState(branding.social_facebook ?? "");
  const [tiktok, setTiktok] = useState(branding.social_tiktok ?? "");
  const [youtube, setYoutube] = useState(branding.social_youtube ?? "");
  const [returnPolicy, setReturnPolicy] = useState(branding.return_policy ?? "");
  const [footerTagline, setFooterTagline] = useState(branding.footer_tagline ?? "");
  const [footerCopyright, setFooterCopyright] = useState(branding.footer_copyright ?? "");
  const [showPoweredBy, setShowPoweredBy] = useState(branding.show_powered_by);

  const onSave = useCallback(() => {
    const body: BrandingUpdateBody = {
      tagline,
      announcement_text: announcementText,
      announcement_link: announcementLink,
      announcement_active: announcementActive,
      social_instagram: instagram,
      social_twitter: twitter,
      social_facebook: facebook,
      social_tiktok: tiktok,
      social_youtube: youtube,
      return_policy: returnPolicy,
      footer_tagline: footerTagline,
      footer_copyright: footerCopyright,
      show_powered_by: showPoweredBy,
    };
    update.mutate(body, {
      onSuccess: () => Alert.alert("Saved", "Your branding was updated."),
      onError: (err) =>
        Alert.alert("Couldn't save", err instanceof ApiError ? err.message : "Please try again."),
    });
  }, [
    tagline,
    announcementText,
    announcementLink,
    announcementActive,
    instagram,
    twitter,
    facebook,
    tiktok,
    youtube,
    returnPolicy,
    footerTagline,
    footerCopyright,
    showPoweredBy,
    update,
  ]);

  return (
    <ScrollView
      contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
      keyboardShouldPersistTaps="handled"
    >
      <Eyebrow label="Storefront" />
      <FieldInput label="Tagline" value={tagline} onChangeText={setTagline} placeholder="A short line under your name" />

      <View style={styles.switchRow}>
        <Text preset="body" color="text">
          Show announcement bar
        </Text>
        <Switch value={announcementActive} onValueChange={setAnnouncementActive} trackColor={{ true: theme.colors.accent }} />
      </View>
      <FieldInput
        label="Announcement text"
        value={announcementText}
        onChangeText={setAnnouncementText}
        placeholder="Free shipping this week"
      />
      <FieldInput
        label="Announcement link"
        value={announcementLink}
        onChangeText={setAnnouncementLink}
        placeholder="https://…"
        autoCapitalize="none"
      />

      <Eyebrow label="Social" style={styles.section} />
      <FieldInput label="Instagram" value={instagram} onChangeText={setInstagram} placeholder="@handle or URL" autoCapitalize="none" />
      <FieldInput label="Twitter / X" value={twitter} onChangeText={setTwitter} placeholder="@handle or URL" autoCapitalize="none" />
      <FieldInput label="Facebook" value={facebook} onChangeText={setFacebook} placeholder="URL" autoCapitalize="none" />
      <FieldInput label="TikTok" value={tiktok} onChangeText={setTiktok} placeholder="@handle or URL" autoCapitalize="none" />
      <FieldInput label="YouTube" value={youtube} onChangeText={setYoutube} placeholder="URL" autoCapitalize="none" />

      <Eyebrow label="Footer & policy" style={styles.section} />
      <FieldInput label="Return policy" value={returnPolicy} onChangeText={setReturnPolicy} placeholder="Your return policy" multiline />
      <FieldInput label="Footer tagline" value={footerTagline} onChangeText={setFooterTagline} placeholder="Shown in the footer" />
      <FieldInput label="Footer copyright" value={footerCopyright} onChangeText={setFooterCopyright} placeholder="© Your store" />
      <View style={styles.switchRow}>
        <Text preset="body" color="text">
          Show &ldquo;Powered by&rdquo;
        </Text>
        <Switch value={showPoweredBy} onValueChange={setShowPoweredBy} trackColor={{ true: theme.colors.accent }} />
      </View>

      <Text preset="caption" color="textTertiary">
        Colours, fonts, logo and SEO are configured on the web dashboard.
      </Text>

      <Pressable
        style={[styles.btn, update.isPending && styles.disabled]}
        onPress={onSave}
        disabled={update.isPending}
        accessibilityRole="button"
        accessibilityLabel="Save branding"
      >
        {update.isPending ? (
          <ActivityIndicator size="small" color={theme.colors.inverse} />
        ) : (
          <Text preset="bodyEmphasis" color="inverse">
            Save changes
          </Text>
        )}
      </Pressable>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  section: { paddingTop: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  switchRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.md,
  },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});
