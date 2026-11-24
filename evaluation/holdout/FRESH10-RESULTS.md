# Fresh independent batch: first evaluation

Ten frozen cases from repositories Ceaser had not been tuned against.
Each case was verified before scoring: base fails the injected regression
test as a test failure and head passes. The engine was not tuned against
these cases before this first evaluation. This is the baseline to compare
against, not a tuning target.

| # | Case | Reviewed culprit | First primary | Exact function |
|---|---|---|---|---|
| 1 | gin-gonic-gin-4852 | getValue | node.getValue | hit |
| 2 | go-chi-chi-1159 | Mount | ABSTAIN | miss |
| 3 | go-chi-chi-1156 | NewCompressor | NewCompressor | hit |
| 4 | go-chi-chi-1148 | routes | NewRouteContext | miss |
| 5 | go-chi-chi-1097 | routeHTTP | NewRouter | miss |
| 6 | labstack-echo-3238 | BindBody | bindData | miss |
| 7 | labstack-echo-3113 | Add | DefaultRouter.Route | miss |
| 8 | golang-jwt-jwt-511 | parseClaimsString | MapClaims.GetAudience | miss |
| 9 | spf13-pflag-511 | parseSingleShortArg | FlagSet.parseSingleShortArg | hit |
| 10 | spf13-pflag-542 | wrap | NewFlagSet | miss |

Exact-function: 3/10.

Do not tune against this batch. Collect a further fresh batch for any
later comparison.

## CI reproduction

Workflow `final-eval.yml` now runs this manifest. Run `45057346470` on
`ceaser-org/ceaser` completed successfully and produced the same counts from the
uploaded `final-mission-eval` artifact: 10 cases, 10 verified, 3 exact-function
hits. The same run reported 3 of 5 on the exposed holdout and 6 of 6 on the
paired development cases with no regressions.

The holdout runner was later corrected to compare the short function name, so
`FlagSet.Count` no longer counts as a hit for culprit `Set`. Recomputing this
batch under the corrected metric still gives 3 of 10.
