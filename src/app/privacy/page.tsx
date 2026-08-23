import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid privacy",
  description: "What VibeGrid stores for private crew play and reliability."
};

export default function PrivacyPage() {
  return (
    <PolicyPage
      eyebrow="Privacy"
      title="VibeGrid privacy"
      intro="VibeGrid uses guest browser identities. It stores only what is needed to run private crew rounds, preserve their history, and keep the service reliable."
      sections={[
        {
          title: "Browser identity",
          body: [
            "VibeGrid does not require a public account. An HttpOnly guest session cookie identifies one browser so it can create or join crews and return to its own cards and votes.",
            "Daily and Unlimited practice boards are fetched from the public service, but your practice selections, titles, and tutorial votes stay in the browser and are not submitted. The browser also remembers that its first-visit introduction was dismissed; that preference contains no card or crew content."
          ]
        },
        {
          title: "Crew rounds",
          body: [
            "For each crew, VibeGrid stores the crew name, invite code, member display names, daily card titles, the four selected fragment ids, blind votes, and timestamps.",
            "A card keeps a snapshot of its author display name so an old result remains understandable after membership changes. Crew content is returned only to current members; invite links should be treated as private capability links."
          ]
        },
        {
          title: "Control and retention",
          body: [
            "A crew owner can rotate a leaked invite and remove a member. A member can leave. When the last member leaves, the crew and its card and vote history are deleted through database cascades.",
            "Otherwise crew history is retained so members can revisit results. There is no claim here that a provider backup or restore schedule exists until that external configuration is verified."
          ]
        },
        {
          title: "Reliability and safety",
          body: [
            "The service may process IP address, request path, status code, latency, user agent, and rate-limit counters in logs or monitoring systems.",
            "Admin access uses a separate revocable HttpOnly session and CSRF protection. Public practice does not require a login, email address, or phone number."
          ]
        }
      ]}
    />
  );
}
