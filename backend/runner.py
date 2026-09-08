import sys
import json

# runner.py — LeetCode Ranked Evaluation Harness
#
# This script is the "judge" that runs inside the Docker sandbox container.
# It is never executed directly by users — it's copied into the container
# alongside their solution.py and our meta_config.json.
#
# FLOW:
#  1. Load meta_config.json to get class name, method name, and test cases.
#  2. Import the user's solution.py module using Python's standard import system.
#  3. Use getattr() (reflection) to find the class and method by name dynamically.
#  4. Run each test case, compare result to expected output.
#  5. sys.exit(0) if all pass, sys.exit(1) on first failure (fail-fast).

def run_judge():
    # ── Step 1: Load judge configuration ─────────────────────────────────────
    try:
        with open('/app/meta_config.json', 'r') as f:
            meta = json.load(f)
    except Exception as e:
        print(f"CRITICAL SYSTEM ERROR: Failed to load test metadata: {e}")
        sys.exit(1)

    class_name  = meta['class_name']
    method_name = meta['method_name']
    test_cases  = meta['test_cases']   # List of {input, expected} dicts

    # ── Step 2: Import the user's solution module ─────────────────────────────
    # Python's import system looks for solution.py in /app (which is on sys.path
    # because runner.py is in the same directory).
    try:
        import solution
        # getattr(solution, class_name) is Python reflection — it retrieves the
        # class object by its string name. This lets us work with any class name
        # without hardcoding it (Two Sum uses "Solution", others might differ).
        user_class  = getattr(solution, class_name)()
        user_method = getattr(user_class, method_name)
    except AttributeError as e:
        print(f"CRITICAL ERROR: Could not find class '{class_name}' or method '{method_name}': {e}")
        sys.exit(1)
    except Exception as e:
        print(f"CRITICAL ERROR: Failed to import solution: {e}")
        sys.exit(1)

    # ── Step 3: Run all test cases sequentially ───────────────────────────────
    passed_count = 0
    total_cases  = len(test_cases)
    print("--- START EVALUATION ENGINE ---")

    for i, case in enumerate(test_cases):
        # Each test case is a dict: {"input": ..., "expected": ...}
        # input may be a list (multiple args) or a single value.
        args     = case['input']
        expected = case['expected']

        # Handle double-stringified JSON inputs/outputs from old PoC database seeds
        # (e.g., converting the string '"true"' into Python boolean True).
        if isinstance(expected, str):
            try:
                expected = json.loads(expected)
            except Exception:
                pass
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except Exception:
                pass

        try:
            if isinstance(args, list):
                result = user_method(*args)   # Unpack list as positional args
            else:
                result = user_method(args)    # Single argument

            # Normalize tuple results to lists for JSON-compatible comparison
            if isinstance(result, tuple):
                result = list(result)

            if result == expected or str(result) == str(expected):
                print(f"Test Case {i+1}/{total_cases}: PASSED")
                passed_count += 1
            else:
                print(f"Test Case {i+1}/{total_cases}: FAILED (Got: {result}, Expected: {expected})")
                print(f"\nSubmission rejected: Failed on Test Case {i+1}.")
                sys.exit(1)  # Fail-fast: stop on first failing test case

        except Exception as e:
            print(f"Test Case {i+1}/{total_cases}: RUNTIME CRASH ({type(e).__name__}: {e})")
            sys.exit(1)

    # ── All test cases passed ─────────────────────────────────────────────────
    print(f"\nResult: ALL {passed_count}/{total_cases} test cases passed!")
    # sys.exit(0) is implicit (Python exits 0 on normal return) but explicit is cleaner.
    sys.exit(0)

if __name__ == '__main__':
    run_judge()
