// PR gate for external contributors, run by .github/workflows/pr-evidence-check.yml.
// The rules it enforces are documented for contributors in CONTRIBUTING.md.
//
// Skipped: bots, maintainers (OWNER / MEMBER / COLLABORATOR, anyone with write
// access, the ALLOWLIST below), PRs carrying the `skip-evidence-check` label,
// and PRs that only touch docs / tests / CI files.
//
// Everyone else must provide:
//   - evidence of self-testing: screenshots uploaded through the GitHub web UI
//     and the "I ran this change on a real Nightingale instance" checkbox ticked
//   - a linked issue (`Fixes #123`) that exists in this repository and has not
//     been declined
//   - a PR within the size and open-PR LIMITS below (a maintainer lifts the
//     size limit alone with the `skip-size-limit` label)
// Authors with fewer than CONTRIBUTOR_MIN_MERGED_PRS merged PRs additionally
// need a maintainer response on that issue, so the change is discussed before
// any code is sent, and get tighter limits.
//
// A failing PR is labeled, gets one explanatory comment and is closed. Editing
// the description re-runs the check, which reopens the PR once it passes.
//
// The checks are decided from the diff, not from the PR type the author ticked.
// Nothing here checks out or runs PR code.

// Explicit trusted usernames (private org members, long-time maintainers).
const ALLOWLIST = ['zbqBaoqing'];
const CONTRIBUTOR_MIN_MERGED_PRS = 3;
// Per author tier: open PRs at a time, and files / changed lines outside
// EXEMPT_FILES in one PR. The newcomer size limit is meant to bite: what
// exceeded it in the past were whole features (new datasources, SSO providers),
// whose scope a maintainer should agree to first and then waive with
// SKIP_SIZE_LABEL. Keep files at 10 or more, typo fixes routinely touch 6-7.
// The contributor limits only catch runaway branches such as #2975 (203 files).
const LIMITS = {
  newcomer: { openPRs: 2, files: 10, lines: 500 },
  contributor: { openPRs: Infinity, files: 50, lines: 10000 },
};

const SKIP_LABEL = 'skip-evidence-check';
// Lifts only the size limit; the issue and evidence requirements still apply.
const SKIP_SIZE_LABEL = 'skip-size-limit';
const NEEDS_ISSUE = 'needs-issue';
const NEEDS_EVIDENCE = 'needs-evidence';
const OVER_LIMIT = 'over-limit';
const LABELS = {
  [NEEDS_ISSUE]: {
    title: 'Issue first',
    description: 'PR has no linked issue that was discussed with a maintainer first',
  },
  [NEEDS_EVIDENCE]: {
    title: 'Evidence of self-testing',
    description: 'PR description is missing the required screenshots / reproduction steps',
  },
  [OVER_LIMIT]: {
    title: 'Contribution limits',
    description: 'PR exceeds the size or open-PR limit for external contributors',
  },
};
const MARKER = '<!-- pr-evidence-check -->';
// Present in the bot comment only while the PR is closed by this check; a PR
// closed by a maintainer is never reopened.
const CLOSED_NOTE = 'closed automatically';

const MAINTAINER_ASSOCIATIONS = ['OWNER', 'MEMBER', 'COLLABORATOR'];
// Only people with triage access can set these, so they count as a maintainer
// response. Never auto-apply any of them from an issue template.
const ACCEPTED_ISSUE_LABELS = ['bug', 'enhancement', 'help wanted', 'good first issue'];
const DECLINED_ISSUE_LABELS = ['not planned', 'wontfix', 'invalid'];
const EXEMPT_FILES = [/\.md$/i, /^doc\//, /_test\.go$/, /(^|\/)testdata\//, /^\.github\//];
// Only the github.com editor produces these URLs: there is no API to upload
// one, so unlike an external link or an image committed to the branch (both
// seen as generated "evidence" in past PRs) a person has to take part.
const ATTACHMENT = /https:\/\/github\.com\/user-attachments\/assets\/[\w-]+/g;

function parseBody(raw, owner, repo) {
  // Template instructions live in HTML comments and are not evidence.
  const body = (raw || '').replace(/<!--[\s\S]*?-->/g, '');
  const checked = (label) => new RegExp(`^\\s*[-*]\\s*\\[[xX]\\]\\s*${label}`, 'm').test(body);
  const issueRef = new RegExp(
    '\\b(?:fix|fixes|fixed|close|closes|closed|resolve|resolves|resolved)\\s*:?\\s*'
    + `(?:(?:${owner}/${repo})?#|https://github\\.com/${owner}/${repo}/issues/)(\\d+)`, 'gi');
  return {
    isBugFix: checked('Bug fix'),
    isFeature: checked('New feature'),
    attachments: new Set(body.match(ATTACHMENT)).size,
    hasVerified: checked('I ran this change on a real Nightingale instance'),
    issues: [...new Set([...body.matchAll(issueRef)].map((m) => Number(m[1])))],
  };
}

async function isMaintainer(user, api) {
  return ALLOWLIST.includes(user.login)
    || MAINTAINER_ASSOCIATIONS.includes(user.association)
    || api.hasWriteAccess(user.login);
}

async function maintainerResponded(issue, api) {
  if (issue.labels.some((name) => ACCEPTED_ISSUE_LABELS.includes(name))) return true;
  for (const user of [issue.author, ...await api.listCommenters(issue.number)]) {
    if (await isMaintainer(user, api)) return true;
  }
  return false;
}

// Decides what a PR is missing. `api` is the read-only view of the repository
// built by githubApi(); returns { reason, problems: [{ label, text }] }.
async function evaluate(pr, api) {
  const pass = (reason) => ({ reason, problems: [] });
  if (pr.labels.includes(SKIP_LABEL)) return pass(`waived with the \`${SKIP_LABEL}\` label`);
  if (pr.isBot) return pass('the author is a bot');
  if (await isMaintainer({ login: pr.author, association: pr.association }, api)) {
    return pass('the author is a maintainer or on the allowlist');
  }
  const files = await api.listFiles(pr.number);
  const code = files.filter((f) => !EXEMPT_FILES.some((re) => re.test(f.path)));
  if (files.length > 0 && code.length === 0) return pass('only docs / tests / CI files changed');

  const desc = parseBody(pr.body, api.owner, api.repo);
  const history = await api.authorPRs(pr.author);
  const newcomer = history.merged < CONTRIBUTOR_MIN_MERGED_PRS;
  const limits = newcomer ? LIMITS.newcomer : LIMITS.contributor;
  const who = newcomer
    ? `authors with fewer than ${CONTRIBUTOR_MIN_MERGED_PRS} merged PRs in this repository`
    : 'external contributors';
  const problems = [];

  const issues = [];
  for (const number of desc.issues.slice(0, 5)) {
    const issue = await api.getIssue(number);
    const declined = issue && (issue.stateReason === 'not_planned'
      || issue.labels.some((name) => DECLINED_ISSUE_LABELS.includes(name)));
    if (issue && !issue.isPR && !declined) issues.push(issue);
  }
  if (desc.issues.length === 0) {
    problems.push({ label: NEEDS_ISSUE,
      text: 'Link the issue this PR addresses, e.g. `Fixes #123`. If there is none, open one and discuss the change there before sending code.' });
  } else if (issues.length === 0) {
    problems.push({ label: NEEDS_ISSUE,
      text: `The linked ${desc.issues.map((n) => `#${n}`).join(', ')} cannot be used: it must be an existing issue in this repository (not a pull request) that has not been declined (\`not planned\` / \`wontfix\` / \`invalid\`).` });
  } else if (newcomer) {
    let discussed = false;
    for (const issue of issues) {
      if (await maintainerResponded(issue, api)) { discussed = true; break; }
    }
    if (!discussed) {
      problems.push({ label: NEEDS_ISSUE,
        text: `${issues.map((i) => `#${i.number}`).join(', ')} has no response from a maintainer yet. Authors with fewer than ${CONTRIBUTOR_MIN_MERGED_PRS} merged PRs in this repository must discuss the change in an issue before sending code. Wait for a maintainer to reply to or label the issue, then edit this PR description to re-run the check.` });
    }
  }

  // A bug fix needs a before and an after.
  if (desc.attachments < (desc.isBugFix ? 2 : 1)) {
    const what = desc.isBugFix
      ? 'Attach at least two screenshots: the problem **before** the fix and the same path **after** the fix (for logs or API output, a screenshot of the terminal), plus the full reproduction steps.'
      : desc.isFeature
        ? 'Attach screenshot(s) of the new page / behavior running in Nightingale.'
        : 'Attach screenshot(s) showing this change running on a real Nightingale instance.';
    problems.push({ label: NEEDS_EVIDENCE,
      text: `${what} Paste or drag the images into the description on github.com; external links and images committed to a branch are not accepted.` });
  }
  if (!desc.hasVerified) {
    problems.push({ label: NEEDS_EVIDENCE,
      text: 'Tick the checkbox confirming you ran this change on a real Nightingale instance yourself.' });
  }

  const lines = code.reduce((sum, f) => sum + f.changes, 0);
  if (!pr.labels.includes(SKIP_SIZE_LABEL) && (code.length > limits.files || lines > limits.lines)) {
    problems.push({ label: OVER_LIMIT,
      text: `This PR changes ${lines} lines in ${code.length} files outside docs and tests; the limit for ${who} is ${limits.lines} lines and ${limits.files} files. Split it into smaller PRs, or agree on the scope in the linked issue and ask a maintainer there to lift the limit with the \`${SKIP_SIZE_LABEL}\` label.` });
  }
  // Only older PRs count, so the first ones an author opened are never the ones closed.
  const older = history.open.filter((number) => number < pr.number);
  if (older.length >= limits.openPRs) {
    problems.push({ label: OVER_LIMIT,
      text: `You already have ${older.length} open PRs (${older.map((n) => `#${n}`).join(', ')}); ${who} can have at most ${limits.openPRs} open at a time. Get those merged or close one, then edit this PR description to re-run the check.` });
  }

  return problems.length > 0 ? { reason: `${problems.length} problem(s)`, problems } : pass('all requirements are met');
}

function githubApi(github, owner, repo) {
  const writeAccess = new Map();
  return {
    owner,
    repo,
    hasWriteAccess(login) {
      if (!writeAccess.has(login)) {
        writeAccess.set(login, github.rest.repos.getCollaboratorPermissionLevel({ owner, repo, username: login })
          .then(({ data }) => ['admin', 'maintain', 'write'].includes(data.permission))
          .catch((e) => { if (e.status === 404) return false; throw e; }));
      }
      return writeAccess.get(login);
    },
    async authorPRs(login) {
      const prs = `repo:${owner}/${repo} is:pr author:${login}`;
      const { merged, open } = await github.graphql(
        `query($merged: String!, $open: String!) {
          merged: search(query: $merged, type: ISSUE) { issueCount }
          open: search(query: $open, type: ISSUE, first: 50) { nodes { ... on PullRequest { number } } }
        }`,
        { merged: `${prs} is:merged`, open: `${prs} is:open` });
      return { merged: merged.issueCount, open: open.nodes.map((n) => n.number) };
    },
    listFiles(number) {
      return github.paginate(github.rest.pulls.listFiles, { owner, repo, pull_number: number, per_page: 100 },
        (res) => res.data.map((f) => ({ path: f.filename, changes: f.changes })));
    },
    async getIssue(number) {
      try {
        const { data } = await github.rest.issues.get({ owner, repo, issue_number: number });
        return {
          number,
          isPR: !!data.pull_request,
          author: { login: data.user?.login, association: data.author_association },
          labels: data.labels.map((l) => l.name),
          stateReason: data.state_reason,
        };
      } catch (e) {
        if (e.status === 404 || e.status === 410) return null;
        throw e;
      }
    },
    listCommenters(number) {
      return github.paginate(github.rest.issues.listComments, { owner, repo, issue_number: number, per_page: 100 },
        (res) => res.data.filter((c) => c.user).map((c) => ({ login: c.user.login, association: c.author_association })));
    },
  };
}

async function run({ github, context, core }) {
  const { owner, repo } = context.repo;
  const pr = context.payload.pull_request;
  const target = { owner, repo, issue_number: pr.number };
  const labels = pr.labels.map((l) => l.name);

  const { reason, problems } = await evaluate({
    number: pr.number,
    author: pr.user.login,
    isBot: pr.user.type === 'Bot',
    association: pr.author_association,
    labels,
    body: pr.body,
  }, githubApi(github, owner, repo));
  core.info(`${problems.length > 0 ? 'failed' : 'passed'}: ${reason}`);

  const stale = labels.filter((name) => name in LABELS);
  const wanted = Object.keys(LABELS).filter((name) => problems.some((p) => p.label === name));
  if (wanted.length === 0 && stale.length === 0 && pr.state === 'open') return;

  const comments = await github.paginate(github.rest.issues.listComments, { ...target, per_page: 100 });
  const comment = comments.find((c) => c.body && c.body.includes(MARKER));
  const closedByUs = pr.state === 'closed' && !!comment && comment.body.includes(CLOSED_NOTE);
  const upsertComment = (text) => {
    const body = `${MARKER}\n${text}`;
    return comment
      ? github.rest.issues.updateComment({ owner, repo, comment_id: comment.id, body })
      : github.rest.issues.createComment({ ...target, body });
  };
  const removeLabel = (name) => github.rest.issues.removeLabel({ ...target, name })
    .catch((e) => { if (e.status !== 404) throw e; });

  // --- pass ---
  if (wanted.length === 0) {
    if (stale.length === 0 && !closedByUs) return;
    let text = `✅ Check passed: ${reason}. A maintainer will review this PR.`;
    if (closedByUs) {
      try {
        await github.rest.pulls.update({ owner, repo, pull_number: pr.number, state: 'open' });
      } catch (e) {
        if (e.status !== 422) throw e;
        text = `✅ Check passed: ${reason}. GitHub could not reopen this PR (its branch was deleted or force-pushed while it was closed, or another PR is open for the same branch); please open a new PR.`;
      }
    }
    for (const name of stale) await removeLabel(name);
    await upsertComment(text);
    return;
  }

  // --- fail ---
  for (const name of wanted) {
    try {
      await github.rest.issues.getLabel({ owner, repo, name });
    } catch (e) {
      if (e.status !== 404) throw e;
      await github.rest.issues.createLabel({ owner, repo, name, color: 'e11d21', description: LABELS[name].description });
    }
  }
  await github.rest.issues.addLabels({ ...target, labels: wanted });
  for (const name of stale) {
    if (!wanted.includes(name)) await removeLabel(name);
  }

  const closing = pr.state === 'open';
  const ours = closing || closedByUs;
  const guide = `https://github.com/${owner}/${repo}/blob/${context.payload.repository.default_branch}/CONTRIBUTING.md`;
  const lines = [
    `Hi @${pr.user.login}, thanks for the PR. It does not meet the [contribution requirements](${guide}) yet${ours ? `, so it has been **${CLOSED_NOTE}**` : ''}:`,
    '',
  ];
  for (const name of wanted) {
    lines.push(`**${LABELS[name].title}**`, ...problems.filter((p) => p.label === name).map((p) => `- ${p.text}`), '');
  }
  lines.push(
    'Every change must be verified by a human on a running Nightingale instance; unit tests alone are not sufficient. AI-assisted PRs are welcome, but the requirements above still apply.',
    '',
    ours
      ? 'Once the points above are resolved, edit the PR description: the check re-runs and reopens the PR. Do not force-push or delete the branch while the PR is closed, otherwise GitHub cannot reopen it.'
      : 'Edit the PR description once the points above are resolved; the check will re-run.',
  );
  await upsertComment(lines.join('\n'));

  if (closing) {
    await github.rest.pulls.update({ owner, repo, pull_number: pr.number, state: 'closed' });
  }
  core.setFailed(`PR does not meet the contribution requirements: ${problems.length} problem(s)`);
}

module.exports = run;
module.exports.evaluate = evaluate;
module.exports.parseBody = parseBody;
