---
description: Investigate why a website is broken in zen-core
argument-hint: |-
  i: The URL to investigate
---
I need to debug why {{i}} is not working correctly with zen-core.
Please follow this plan:

1. **Production Test**: Use `productionTester` to fetch {{i}} and see the actual response code and errors.
2. **Rule Test**: Use `ruleTester` to check if any rules in `networkrules` are blocking {{i}}.
3. **Log Check**: Use `logViewer` to search for "BLOCK" or "ERROR" related to this domain in the zen-core logs.
4. **Analysis**: Provide specific instructions on how to modify the Go code if a false positive is found.