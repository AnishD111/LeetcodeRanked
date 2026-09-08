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
    inputs = [json.loads(x) for x in meta['test_inputs']]
    expected_outputs = [json.loads(x) for x in meta['test_outputs']]
    
    # Import user submission module
    try:
        import solution
        user_class = getattr(solution, class_name)()
        user_method = getattr(user_class, method_name)
    except Exception as e:
        print(f"CRITICAL ERROR: Could not find class '{class_name}' or method '{method_name}': {e}")
        sys.exit(1)
        
    passed_count = 0
    total_cases = len(inputs)
    print("--- START EVALUATION ENGINE ---")
    
    for i, (args, expected) in enumerate(zip(inputs, expected_outputs)):
        try:
            if isinstance(args, list):
                result = user_method(*args)
            else:
                result = user_method(args)
                
            if result == expected:
                print(f"Test Case {i+1}/{total_cases}: PASSED")
                passed_count += 1
            else:
                print(f"Test Case {i+1}/{total_cases}: FAILED (Got: {result}, Expected: {expected})")
                print(f"\n❌ Submission rejected: Failed on Test Case {i+1}.")
                sys.exit(1) # Stop immediately on first failure
        except Exception as e:
            print(f"Test Case {i+1}/{total_cases}: RUNTIME CRASH ({type(e).__name__}: {e})")
            sys.exit(1)
            
    print(f"\nResult: ALL {passed_count}/{total_cases} test cases passed!")

if __name__ == '__main__':
    run_judge()