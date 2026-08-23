import { PolicyPage } from "@/components/PolicyPage";

export const metadata = {
  title: "VibeGrid terms",
  description: "Plain-language terms for making and judging VibeGrid cards with a private crew."
};

export default function TermsPage() {
  return (
    <PolicyPage
      eyebrow="Terms"
      title="VibeGrid terms"
      intro="These are plain-language launch terms for making and judging daily vibe cards with people you invite."
      sections={[
        {
          title: "Using VibeGrid",
          body: [
            "You can use the public practice round or create and join private crews. Do not interfere with the service, bypass rate limits, scrape aggressively, or use the app to target or harm another person.",
            "The public daily and Unlimited practice ballots use clearly labeled house cards to demonstrate the loop. They are fixtures, not claims about real players or live community activity. Unlimited deals may eventually repeat finite curated source material.",
            "A crew invite is a capability link: anyone who has it may ask to join until the owner rotates it. Share it only with people you intend to invite."
          ]
        },
        {
          title: "Your cards",
          body: [
            "You are responsible for the display name and card titles you submit. Do not use them for harassment, hate, threats, sexual content involving minors, private information, impersonation, or material you do not have the right to share.",
            "By submitting a card, you allow VibeGrid to show it to current members of that crew, tally its votes, and preserve it in that crew’s result history."
          ]
        },
        {
          title: "Crew control",
          body: [
            "Crew owners can remove members and rotate invites. Members can leave. VibeGrid may remove content or restrict access when needed for safety, legal compliance, or service integrity.",
            "Blind judging hides authors only during the ballot. It is a game mechanic, not an anonymity guarantee: results reveal the author display name to crew members."
          ]
        },
        {
          title: "Availability",
          body: [
            "VibeGrid is provided as a game service without a guarantee that every board, crew, link, feature, or result will always be available.",
            "We may change the product, limit usage, or pause features to protect reliability, safety, or legal compliance."
          ]
        }
      ]}
    />
  );
}
