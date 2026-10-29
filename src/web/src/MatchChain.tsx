type Resolution = { inputName: string; resolved: boolean; tagUrn?: string };

type Props = {
  boughtNames: string[];
  cohortSize?: number;
  resolutions: Resolution[];
  placeName: string;
  affinity?: number;
  crossDomainCategories?: string[];
};

/** Owner-facing proof of how Qloo matching led to this place (not the LLM pitch). */
export function MatchChain({
  boughtNames,
  cohortSize,
  resolutions,
  placeName,
  affinity,
  crossDomainCategories,
}: Props) {
  const tags = resolutions.filter((r) => r.resolved && r.tagUrn).map((r) => r.inputName);
  const fit =
    affinity !== undefined && Number.isFinite(affinity)
      ? `${placeName} · Qloo fit ${affinity.toFixed(2)}`
      : placeName;

  const partnerLine =
    crossDomainCategories && crossDomainCategories.length > 0
      ? `Qloo surfaced nearby: ${crossDomainCategories.join(", ")} (not your business type)`
      : "Qloo ranked nearby places that fit your customers' tastes, excluding your business type";

  const cohortLead =
    cohortSize !== undefined && cohortSize > 0
      ? `Top ${cohortSize} customers by value · bought: `
      : "";
  const boughtBody =
    cohortLead + (boughtNames.join(", ") || "—");

  const steps = [
    { n: 1, title: "Your valuable customers", body: boughtBody },
    { n: 2, title: "Qloo taste tags", body: tags.length ? tags.join(", ") : "—" },
    { n: 3, title: "Cross-category matches", body: partnerLine },
    { n: 4, title: "Qloo ranked this place", body: fit },
  ];

  return (
    <div className="match-chain" aria-label="How this match was made">
      <p className="match-chain-heading">How this match was made</p>
      <ol className="match-chain-steps">
        {steps.map((s) => (
          <li key={s.n} className="match-chain-step">
            <span className="match-chain-num" aria-hidden="true">
              {s.n}
            </span>
            <div>
              <p className="match-chain-title">{s.title}</p>
              <p className="match-chain-body">{s.body}</p>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

export function affinityForCandidate(
  candidateId: string | undefined,
  qlooEvidence: { id: string; affinity: number }[] | undefined,
): number | undefined {
  if (!candidateId || !qlooEvidence) return undefined;
  const c = qlooEvidence.find((e) => e.id === candidateId);
  return c?.affinity;
}
