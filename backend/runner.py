import sys
import json

def run_judge():
    try:
        with open('/app/meta_config.json', 'r') as f:
            meta = json.load(f)
    except Exception as e:
        print(f"CRITICAL SYSTEM ERROR: Failed to load test metadata: {e}")
        sys.exit(1)

    class_name = meta['class_name']
    method_name = meta['method_name']
    test_cases = meta['test_cases']

    try:
        import solution
        user_class = getattr(solution, class_name)()
        user_method = getattr(user_class, method_name)
    except AttributeError as e:
        print(f"CRITICAL ERROR: Could not find class '{class_name}' or method '{method_name}': {e}")
        sys.exit(1)
    except Exception as e:
        print(f"CRITICAL ERROR: Failed to import solution: {e}")
        sys.exit(1)

    passed_count = 0
    total_cases = len(test_cases)
    print("--- START EVALUATION ENGINE ---")

    for i, case in enumerate(test_cases):
        args = case['input']
        expected = case['expected']

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
                result = user_method(*args)
            else:
                result = user_method(args)

            if isinstance(result, tuple):
                result = list(result)

            if result == expected or str(result) == str(expected):
                print(f"Test Case {i+1}/{total_cases}: PASSED")
                passed_count += 1
            else:
                print(f"Test Case {i+1}/{total_cases}: FAILED (Got: {result}, Expected: {expected})")
                print(f"\nSubmission rejected: Failed on Test Case {i+1}.")
                sys.exit(1)

        except Exception as e:
            print(f"Test Case {i+1}/{total_cases}: RUNTIME CRASH ({type(e).__name__}: {e})")
            sys.exit(1)

    print(f"\nResult: ALL {passed_count}/{total_cases} test cases passed!")
    sys.exit(0)

if __name__ == '__main__':
    run_judge()
