# ferry

ferry turns a plain summary of a messy situation into an analogy you can say out loud.

Good managers explain problems with comparisons anyone can picture: "we keep topping off the oil
instead of fixing the leak." ferry is for the times you understand a problem but cannot find the
words that make other people see it. Describe the situation in plain language, and ferry gives
back two or three sentences with the same cause and effect as the real problem, drawn from a
world your listener already knows and pointed at the decision you need.

## Install

Install from a clone of this repository:

```sh
go install .
```

## Credentials

ferry calls Claude through the Anthropic Go SDK, which finds credentials on its own. Export
`ANTHROPIC_API_KEY`, or run `ant auth login` once and ferry uses that profile.

## Usage

```sh
# Give the summary as arguments, quoted so the shell keeps it in one piece.
ferry "A memory leak crashes the service every night. We restart it each morning instead of
fixing it, and the fix needs one engineer for a week."

# Read it from a file, aim it at a listener, and name a world you prefer.
ferry --file situation.txt --audience "the finance team" --world sports

# Pipe it in and get JSON for another tool.
pbpaste | ferry --json --pretty

# See the skeleton and every candidate, kept or dropped.
ferry --show-work --file situation.txt
```

Run ferry with no summary at a terminal and it asks for one. Type it, then press Ctrl-D. ferry
has no subcommands, so every argument is summary text, and a summary that starts with "help" or
"version" runs like any other. The summary has to be UTF-8 text of 20,000 bytes or less. ferry
sends all of it or none of it, and it turns away a summary that holds one of the tags it uses to
mark its own text in a prompt, such as `</summary>`.

The output looks like this:

```text
Say it like this:
We keep topping off the oil every morning instead of fixing the leak. The engine
still runs, but each quart costs a little more, and one day the leak wins. Let's
put the car on the lift this week.

What stands for what:
- topping off the oil -> restarting the service each morning
- the leak -> the memory bug
- putting the car on the lift -> one engineer for a week on the fix

Where it breaks:
A leak drips at a steady rate, but our crashes come in bursts after big
releases.

World: cars
```

At a terminal, ferry also prints each step on stderr as it happens. Away from a terminal it
stays quiet, apart from a warning when a draft or a check fails. When the analogy it prints is
not a polished one that met every rule, it says why in a line on stderr that starts with
`ferry: note:`, and `--json` output carries the same text in `note`.

## Flags

| Flag          | Default            | What it does                                                       |
|---------------|--------------------|--------------------------------------------------------------------|
| `--audience`  | see below          | Who will hear the analogy, e.g. "my director" or "new engineers".  |
| `--world`     | none               | A world you prefer. It is drafted too, and it wins if it holds up. |
| `--model`     | `claude-fable-5-2` | Claude model ID.                                                   |
| `--effort`    | `high`             | Reasoning effort: `low`, `medium`, `high`, `xhigh`, or `max`.      |
| `--file`      | none               | Read the summary from a file. `-` means stdin.                     |
| `--json`      | off                | Print the whole result as JSON, work included.                     |
| `--pretty`    | off                | Indent the `--json` output.                                        |
| `--show-work` | off                | Also print the skeleton, every candidate, and each verdict.        |
| `--timeout`   | `5m`               | Time limit for one model call, retries included.                   |
| `--log-level` | `warn`             | Lowest log level written to stderr.                                |
| `--version`   | off                | Print the version and stop.                                        |

Every flag but `--version` can also come from the environment as `FERRY_<FLAG>`, e.g.
`FERRY_AUDIENCE` or `FERRY_LOG_LEVEL`. A flag on the command line wins over the environment. A
value that does not parse is an error in either place.

`--world` takes up to six worlds. Repeat it or separate worlds with commas. With no `--audience`,
ferry writes for smart listeners who do not know the details.

`--timeout` covers one whole model call. A rate limit or a server error is retried inside that
time, and a call that runs out of time is not sent again.

`--log-level debug` prints one line for each model call with its step, model, stop reason, token
counts, and time taken. No log line ever holds your summary, a prompt, or a reply.

## How it works

ferry does not ask for a metaphor in one shot, which tends to return limp results like "it's
like building a house." It runs four steps instead, then checks its own final text.

First it reads the situation and pulls out its skeleton: the players, the constraint, the
tradeoff, what happens if nobody acts, the decision, the point you want to land, and the chain
of cause and effect.

Next it drafts several candidates at once, each from a different familiar world. The defaults
are cars, kitchens, construction, and sports. A world you name with `--world` is drafted
alongside them, and it wins over the others if it holds up, even at a lower score.

Then it checks each candidate against the skeleton on its own. A candidate stays only if its
cause and effect have the same shape as the real problem, your listener knows its world, it
points at the decision, and it scores at least 3 of 5 for fit. If none survive, ferry drafts
from a second set of worlds before it gives up.

Then it polishes the winner into two or three sentences you can say out loud, with a list of
what stands for what and the place where the comparison breaks down. Those two parts always
print. They cover you when someone pushes on the comparison, and over time they teach the skill.

Last, it runs the same check on the polished text, because a rewrite can break what the draft
got right. Every analogy ferry prints has passed the check. If the polish fails, or the polished
text does not pass, ferry prints the winning draft as it was checked and says so in a note.

## Privacy

Your summary leaves your machine and goes to Claude. Before you describe anything from your day
job, remove names, company names, and other specifics. ferry prints this reminder on every run,
but it cannot tell what is sensitive and does not scrub anything for you.

## Cost

A normal run makes 11 model calls: one to read the situation, four drafts, four checks, one to
polish the winner, and one to check the polished text. Each `--world` adds a draft and a check,
so two more calls. A run where no draft holds up in the first round drafts and checks a second
set of worlds, which comes to about 17 calls if nothing holds up there either. A malformed reply
costs one retry.

Most of the cost is the model's thinking, which grows with `--effort`. Lower `--effort` to spend
less on each call. At `xhigh` and `max`, ferry doubles the room each reply gets, from 16,000 to
32,000 tokens, since thinking counts against it. A reply that still runs out of room is an
error that tells you to lower `--effort`.

A run that finds no analogy still prints the candidates it paid for and why each was dropped.

## Exit codes

| Code | Meaning                                                                 |
|------|-------------------------------------------------------------------------|
| 0    | An analogy was printed.                                                 |
| 1    | A general failure: a network or file error, or no credentials found.    |
| 2    | A usage problem: a bad flag or setting, or a summary ferry cannot send. |
| 3    | No drafted analogy held up. The candidates and the reasons are printed. |
| 4    | Claude declined the request.                                            |
| 5    | Claude would not accept your credentials.                               |
| 130  | Interrupted with Ctrl-C.                                                |

On code 3, the candidates print as text, or as the partial result with `--json`.
