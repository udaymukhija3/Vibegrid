import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid privacy",
  description: "What VibeGrid stores for gameplay, reports, moderation, and reliability."
};

export default function PrivacyPage() {
  return (
    <PolicyPage
      eyebrow="Privacy"
      title="VibeGrid privacy"
      intro="VibeGrid stores the minimum data needed to run gameplay, community sharing, moderation, and production reliability."
      sections={[
        {
          title: "Gameplay data",
          body: [
            "The app uses a browser session cookie to keep your attempt state, streak, guesses, completion status, and puzzle statistics connected across requests.",
            "Your browser may also store local attempt data so a board can recover after refresh."
          ]
        },
        {
          title: "Community puzzles",
          body: [
            "When you create a community grid, VibeGrid stores the puzzle words, group names, explanations, difficulty, status, and share id.",
            "Community grids are playable by direct link. They do not enter the daily puzzle or public archive unless the product changes in the future."
          ]
        },
        {
          title: "Reports and appeals",
          body: [
            "Reports store the puzzle id, reason code, optional details, optional contact field, browser session id, status, and resolution note.",
            "Appeals store the puzzle id, optional contact field, appeal message, status, and resolution note.",
            "Admin moderation actions are stored in an audit log with actor, action, reason, note, puzzle id, and timestamp."
          ]
        },
        {
          title: "Reliability and safety",
          body: [
            "The service may process IP address, request path, status code, latency, user agent, and rate-limit counters in logs or monitoring systems.",
            "Admin access uses an HttpOnly session cookie. Public gameplay does not require an account."
          ]
        },
        {
          title: "Retention",
          body: [
            "Puzzle, attempt, report, appeal, audit, and rate-limit data is kept while needed to operate the product, investigate abuse, or preserve service integrity.",
            "Contact information entered in a report or appeal is optional and should only be included if you want a moderator to be able to respond."
          ]
        }
      ]}
    />
  );
}
