import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid terms",
  description: "Plain-language terms for playing and sharing VibeGrid puzzles."
};

export default function TermsPage() {
  return (
    <PolicyPage
      eyebrow="Terms"
      title="VibeGrid terms"
      intro="These are plain-language launch terms for playing the daily puzzle and sharing community grids."
      sections={[
        {
          title: "Using VibeGrid",
          body: [
            "You can play daily puzzles, create community puzzles, and share direct links. Do not interfere with the service, bypass rate limits, scrape aggressively, or use the app to harm other people.",
            "Community puzzles are unlisted by default. Anyone with the link can play them unless the grid is removed."
          ]
        },
        {
          title: "Your submissions",
          body: [
            "You are responsible for the words, group names, explanations, reports, and appeals you submit.",
            "By creating a community grid, you allow VibeGrid to host it, display it, share it by link, moderate it, and store the records needed to run the service."
          ]
        },
        {
          title: "Moderation",
          body: [
            "We may remove, archive, reinstate, or decline to host community puzzles that break the community rules, create operational risk, or receive credible reports.",
            "Moderation decisions can be appealed from the unavailable shared puzzle page when the puzzle id still exists."
          ]
        },
        {
          title: "Availability",
          body: [
            "VibeGrid is provided as a game service without a guarantee that every puzzle, link, feature, or statistic will always be available.",
            "We may change the product, limit usage, or pause features to protect reliability, safety, or legal compliance."
          ]
        }
      ]}
    />
  );
}
