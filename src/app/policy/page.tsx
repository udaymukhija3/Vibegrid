import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid community rules",
  description: "What can be shared in community VibeGrid puzzles."
};

export default function CommunityPolicyPage() {
  return (
    <PolicyPage
      eyebrow="Community rules"
      title="Keep shared grids fair and safe"
      intro="Community puzzles are made by players and shared by direct link. These rules explain what can be reported, removed, appealed, or reinstated."
      sections={[
        {
          title: "What is not allowed",
          body: [
            "Do not post hateful, harassing, threatening, sexually explicit, violent, or illegal content.",
            "Do not include private personal information, doxxing hints, passwords, keys, payment details, or other sensitive data.",
            "Do not use community grids for spam, scams, impersonation, malware, phishing, or repeated low-quality posting.",
            "Do not upload content you do not have the right to share, including copyrighted word sets copied from another puzzle without permission.",
            "Do not make grids that target a private person, reveal hidden personal facts, or require inside knowledge meant to embarrass someone."
          ]
        },
        {
          title: "Fair puzzle standard",
          body: [
            "A community grid should be playable from the visible words alone. Tricky is fine; deliberately impossible, misleading, or broken grids may be removed.",
            "Repeated tiles, empty groups, and overlong wording are blocked before saving. Moderators can still remove a grid if the saved puzzle violates these rules."
          ]
        },
        {
          title: "Reports and review",
          body: [
            "Players can report a grid from the puzzle screen without logging in. Reports include a reason code, optional details, the puzzle id, and an optional contact field.",
            "Moderators can dismiss a report, archive the grid, leave a resolution note, and review the audit log for each moderation action."
          ]
        },
        {
          title: "Appeals",
          body: [
            "If a shared grid is unavailable, the puzzle page shows an appeal form. The appeal goes to the admin moderation queue.",
            "Moderators can close an appeal or reinstate the grid. Reinstatement and appeal decisions are recorded in the audit log."
          ]
        }
      ]}
    />
  );
}
