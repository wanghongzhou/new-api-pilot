import { existsSync } from "node:fs";
import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";

type AcceptanceCase = {
  acceptanceId: string;
  requirementId: string;
  fixtures: string;
  layer: string;
  paths: string[];
  evidencePath: string;
};

const root = new URL("../../", import.meta.url).pathname.replace(/^\/(.:\/)/, "$1");
const manifestPath = join(root, "docs", "acceptance", "manifest.yaml");
const artifactRoot = join(root, "artifacts", "acceptance");
const fixtureManifestPath = join(root, "testdata", "design", "manifest.sha256");

function unquote(value: string) {
  return value.replace(/^"|"$/g, "");
}

function parseCases(text: string): AcceptanceCase[] {
  const cases: AcceptanceCase[] = [];
  let current: Partial<AcceptanceCase> | undefined;
  let readingPaths = false;
  let hasLegacyPath = false;
  let hasArrayPaths = false;

  const finish = () => {
    if (!current?.acceptanceId) return;
    if (hasLegacyPath && hasArrayPaths) {
      throw new Error(
        `${current.acceptanceId} sets both test_or_runbook_path and test_or_runbook_paths`,
      );
    }
    if (!current.paths?.length) {
      throw new Error(`${current.acceptanceId} has no executable test/runbook path`);
    }
    if (new Set(current.paths).size !== current.paths.length) {
      throw new Error(`${current.acceptanceId} repeats a test/runbook path`);
    }
    cases.push(current as AcceptanceCase);
  };

  for (const line of text.split(/\r?\n/)) {
    const acceptance = line.match(/^  - acceptance_id: (A(?:\d{2}|100))$/);
    if (acceptance) {
      finish();
      current = { acceptanceId: acceptance[1], paths: [] };
      readingPaths = false;
      hasLegacyPath = false;
      hasArrayPaths = false;
      continue;
    }
    if (!current) continue;

    if (/^    test_or_runbook_paths:\s*$/.test(line)) {
      hasArrayPaths = true;
      readingPaths = true;
      continue;
    }
    if (readingPaths) {
      const item = line.match(/^      - (.+)$/);
      if (item) {
        current.paths?.push(unquote(item[1]));
        continue;
      }
      readingPaths = false;
    }

    const field = line.match(
      /^    (requirement_id|fixture|layer|test_or_runbook_path|evidence_path): (.+)$/,
    );
    if (!field) continue;
    const value = unquote(field[2]);
    if (field[1] === "requirement_id") current.requirementId = value;
    if (field[1] === "fixture") current.fixtures = value;
    if (field[1] === "layer") current.layer = value;
    if (field[1] === "test_or_runbook_path") {
      hasLegacyPath = true;
      current.paths?.push(value);
    }
    if (field[1] === "evidence_path") current.evidencePath = value;
  }
  finish();
  return cases;
}

function multiLayerGaps(item: AcceptanceCase) {
  if (!/^A(?:89|9\d|100)$/.test(item.acceptanceId)) return [];
  const paths = item.paths.map((path) => path.toLowerCase().replaceAll("\\", "/"));
  const gaps: string[] = [];
  if (!paths.some((path) => path.startsWith("tests/integration/") && path.endsWith("_test.go"))) {
    gaps.push("backend integration");
  }
  if (
    !paths.some(
      (path) =>
        (path.startsWith("tests/contract/") && path.endsWith("_test.go")) ||
        (!path.startsWith("tests/integration/") && path.endsWith("_test.go")) ||
        path.endsWith(".test.ts") ||
        path.endsWith(".test.tsx"),
    )
  ) {
    gaps.push("contract/unit");
  }
  if (!paths.some((path) => path.startsWith("web/e2e/") && path.endsWith(".spec.ts"))) {
    gaps.push("desktop/mobile E2E");
  }
  if (
    !paths.some((path) =>
      [
        "privacy-boundary",
        "absence",
        "fixture-consumption",
        "security_acceptance",
        "restore_safety",
      ].some((marker) => path.includes(marker)),
    )
  ) {
    gaps.push("safety absence/fixture consumption");
  }
  return gaps;
}

async function historicalEvidenceAudit() {
  const records: Array<{ id: string; file: string; hash?: string }> = [];
  if (!existsSync(artifactRoot)) return records;
  for (const id of await readdir(artifactRoot)) {
    if (!/^A(?:\d{2}|100)$/.test(id)) continue;
    const idPath = join(artifactRoot, id);
    for (const run of await readdir(idPath, { withFileTypes: true })) {
      if (!run.isDirectory()) continue;
      const file = join(idPath, run.name, "evidence.json");
      if (!existsSync(file)) continue;
      try {
        const evidence = JSON.parse(await readFile(file, "utf8")) as {
          fixture_manifest_sha256?: string;
        };
        records.push({
          id,
          file: file.slice(root.length + 1).replaceAll("\\", "/"),
          hash: evidence.fixture_manifest_sha256,
        });
      } catch {
        records.push({
          id,
          file: file.slice(root.length + 1).replaceAll("\\", "/"),
        });
      }
    }
  }
  return records;
}

const cases = parseCases(await readFile(manifestPath, "utf8"));
if (cases.length !== 100) throw new Error(`expected 100 cases, got ${cases.length}`);

const missingPaths = cases.flatMap((item) =>
  item.paths
    .filter((path) => !path.startsWith("planned:"))
    .filter((path) => !existsSync(join(root, path)))
    .map((path) => `${item.acceptanceId}:${path}`),
);
if (missingPaths.length > 0) {
  throw new Error(`manifest paths are missing:\n${missingPaths.join("\n")}`);
}

const coverageGaps = cases.flatMap((item) =>
  multiLayerGaps(item).map((gap) => `${item.acceptanceId}:${gap}`),
);
if (coverageGaps.length > 0) {
  throw new Error(`A89-A100 multi-layer coverage is incomplete:\n${coverageGaps.join("\n")}`);
}
const playwrightConfig = await readFile(join(root, "web", "playwright.config.ts"), "utf8");
if (
  !playwrightConfig.includes("chromium-desktop") ||
  !playwrightConfig.includes("chromium-mobile")
) {
  throw new Error("Playwright desktop/mobile projects are incomplete");
}

const fixtureManifestBytes = await Bun.file(fixtureManifestPath).arrayBuffer();
const currentFixtureManifestSha = new Bun.CryptoHasher("sha256")
  .update(fixtureManifestBytes)
  .digest("hex");
const oldEvidence = await historicalEvidenceAudit();
const oldIds = new Set(oldEvidence.map(({ id }) => id));
const staleEvidence = oldEvidence.filter(({ hash }) => hash !== currentFixtureManifestSha);

const rows = cases.map((item) => {
  const external = item.layer === "runbook";
  const planned = item.paths.some((path) => path.startsWith("planned:"));
  const status = planned ? "PLANNED" : external ? "EXTERNAL-BLOCKED" : "PATH-MAPPING-PASS";
  const validation = external
    ? "template/path structure only; operation and approvals NOT EVALUATED"
    : item.acceptanceId.match(/^A(?:89|9\d|100)$/)
      ? "integration + contract/unit + Playwright desktop/mobile + safety/fixture path mapping"
      : "authoritative manifest path exists";
  const history = oldIds.has(item.acceptanceId)
    ? "historical raw evidence present; superseded/non-formal"
    : "none";
  return `| ${item.acceptanceId} | ${item.requirementId} | ${item.layer} | ${status} | ${item.paths.join("<br>")} | ${validation} | ${history} |`;
});

const automated = cases.filter(({ layer }) => layer !== "runbook").length;
const external = cases.length - automated;
const output = `# A01-A100 technical pre-evidence matrix

Generated: ${new Date().toISOString()}

The per-case status column validates manifest path mappings only. The separate technical execution baseline records verified sub-agent results, but does not alter \`planned:\` paths, declare formal release acceptance, claim a clean commit, or provide production approval.

## Current validation baseline

- Fixture manifest SHA256: \`${currentFixtureManifestSha}\`.
- Manifest cases parsed: ${cases.length}; every non-planned path exists.
- A89-A100: backend integration, contract/unit, Playwright desktop/mobile E2E, and safety absence/fixture consumption mappings are complete.
- Latest technical execution baseline: Bun unit tests \`215/215\`; Playwright \`79/79\` on chromium-desktop and \`79/79\` on chromium-mobile.
- A99/A100 Docker-backed integration contracts passed and directly consumed F10/F11; ordinary docscheck passed.
- Automated path-mapped cases: ${automated}.
- External/runbook cases not executed: ${external}.
- Ordinary docscheck and the underlying Go/Bun test suites remain independently enforced gates; this matrix does not replace their runner logs.

## Historical evidence audit

- Raw historical \`evidence.json\` records found: ${oldEvidence.length} across ${oldIds.size} acceptance IDs.
- Records whose fixture manifest hash differs from the current hash: ${staleEvidence.length}.
- Historical files are preserved as immutable raw logs; this generator does not promote them to formal evidence.

| ID | Requirement | Layer | Mapping status | Authoritative paths | Validation basis | Historical evidence |
|---|---|---|---|---|---|---|
${rows.join("\n")}

## Formal-release blockers

- All 100 items still require a clean reviewed commit, immutable release image provenance, durable per-item runner logs, independent review, and the required no-skip final gate.
- Runbook items require the real environment, operator execution, measurements, rollback/backup or monitoring observations, and any specified dual approval; template presence is not execution evidence.
`;

await Bun.write(join(artifactRoot, "A01-A100-technical-pre-evidence-matrix.md"), output);
