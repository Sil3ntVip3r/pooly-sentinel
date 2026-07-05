# Resource Thresholds

Task 4 does not evaluate thresholds.

Collectors emit typed observations and metrics only. Task 6 rule-engine work may use these metrics to evaluate WARN, FAIL, or CRITICAL states, but that logic remains outside the collectors.

This separation keeps collectors side-effect free:

- no Discord messages
- no incidents
- no service restarts
- no security repair
- no production scheduler
