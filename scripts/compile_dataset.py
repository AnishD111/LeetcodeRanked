"""
compile_dataset.py — LeetCode Ranked Problem Compiler

Downloads and merges two datasets:
  1. noworneverev/leetcode-api  (titles, descriptions, multi-language snippets)
  2. newfacade/LeetCodeDataset  (30-100+ test cases per problem)

Filters out:
  - Tree/LinkedList/Graph problems (require custom class infrastructure)
  - Corrupted test cases (output is an error message)
  - Unparseable input formats (no '=' sign)

Outputs: compiled_problems.json
"""

import ast
import json
import re
import sys
import urllib.request
import argparse
import warnings
from typing import Any, Optional

# Suppress SyntaxWarning from ast.literal_eval on strings with invalid escape
# sequences (e.g. "\p", "\/") that appear in the HF dataset.
warnings.filterwarnings('ignore', category=SyntaxWarning)


# ─────────────────────────────────────────────────────────────────────────────
# DOWNLOAD HELPERS
# ─────────────────────────────────────────────────────────────────────────────

def download_leetcode_api() -> list[dict]:
    """Download the full questions database from noworneverev/leetcode-api."""
    url = "https://raw.githubusercontent.com/noworneverev/leetcode-api/main/data/leetcode_questions.json"
    print(f"[1/2] Downloading leetcode-api questions from GitHub...")
    req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    with urllib.request.urlopen(req) as response:
        data = json.loads(response.read().decode('utf-8'))
    print(f"       -> Got {len(data)} entries")
    return data


def download_hf_dataset() -> list[dict]:
    """Download all rows from newfacade/LeetCodeDataset via HF API (paginated)."""
    base_url = ("https://datasets-server.huggingface.co/rows?"
                "dataset=newfacade%2FLeetCodeDataset&config=default&split=train"
                "&offset={}&length=100")
    all_rows = []
    offset = 0
    print(f"[2/2] Downloading LeetCodeDataset from Hugging Face...")
    while True:
        url = base_url.format(offset)
        req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
        try:
            with urllib.request.urlopen(req) as response:
                data = json.loads(response.read().decode('utf-8'))
            batch = [r['row'] for r in data['rows']]
            if not batch:
                break
            all_rows.extend(batch)
            offset += 100
            # Print progress every 500
            if len(all_rows) % 500 < 100:
                print(f"       -> {len(all_rows)} rows downloaded...")
        except Exception as e:
            print(f"       -> Stopped at offset {offset}: {e}")
            break
    print(f"       -> Got {len(all_rows)} total rows")
    return all_rows


# ─────────────────────────────────────────────────────────────────────────────
def sanitize_for_json(obj: Any) -> Any:
    """
    Recursively convert non-JSON-serializable Python objects into JSON-safe types.
    Handles: Ellipsis, tuples, sets, bytes, and nested structures.
    """
    if obj is ...:
        return None
    if isinstance(obj, tuple):
        return [sanitize_for_json(x) for x in obj]
    if isinstance(obj, set):
        return [sanitize_for_json(x) for x in sorted(obj)]
    if isinstance(obj, bytes):
        return obj.decode('utf-8', errors='replace')
    if isinstance(obj, list):
        return [sanitize_for_json(x) for x in obj]
    if isinstance(obj, dict):
        return {str(k): sanitize_for_json(v) for k, v in obj.items()}
    # int, float, str, bool, None are all JSON-safe
    return obj


def parse_input_string(input_str: str) -> Optional[list]:
    """
    Parse a HuggingFace test case input string into a list of positional arguments.

    Examples:
        "nums = [3,3], target = 6"       -> [[3,3], 6]
        "s = \"abcabcbb\""               -> ["abcabcbb"]
        "matrix = [[1,2],[3,4]], k = 2"  -> [[[1,2],[3,4]], 2]

    Strategy:
        1. Find all 'varName = ' patterns to locate assignment boundaries.
        2. Extract the value portion of each assignment.
        3. Use ast.literal_eval() for safe Python literal parsing.
    """
    if not input_str or '=' not in input_str:
        return None

    # Find all variable assignment starts: "varname = " preceded by start-of-string or ", "
    # Pattern matches: identifier followed by ' = '
    # We need to find these at the "top level" (not inside strings/brackets)
    assignments = []
    
    # Use regex to find potential assignment positions
    pattern = re.compile(r'(?:^|, )(\w+)\s*=\s*')
    
    # But we need to be careful about matches inside string values.
    # Strategy: find all matches, then validate by checking bracket/quote depth
    # at each match position.
    matches = list(pattern.finditer(input_str))
    
    if not matches:
        return None

    for i, match in enumerate(matches):
        # Value starts after the '= '
        value_start = match.end()
        
        # Value ends at the start of the next match's ', ' separator,
        # or at the end of the string
        if i + 1 < len(matches):
            next_match = matches[i + 1]
            # The next match includes the ', ' prefix — value ends before that
            value_end = next_match.start()
            # Strip trailing ', ' from value
            value_str = input_str[value_start:value_end].rstrip(', ')
        else:
            value_str = input_str[value_start:].strip()

        # Parse the value using ast.literal_eval (safe — no code execution)
        try:
            value = ast.literal_eval(value_str)
            value = sanitize_for_json(value)
        except (ValueError, SyntaxError):
            # If ast.literal_eval fails, treat as raw string
            value = value_str
        
        assignments.append(value)

    return assignments


def parse_output_string(output_str: str) -> Any:
    """
    Parse a HuggingFace test case output string into a Python value.
    
    Tries ast.literal_eval first, falls back to raw string.
    """
    if output_str is None:
        return None

    output_str = output_str.strip()
    
    if not output_str:
        return None

    try:
        result = ast.literal_eval(output_str)
        result = sanitize_for_json(result)
        return result
    except (ValueError, SyntaxError):
        # Fall back to raw string (e.g. "abc", "hello", "XLIV")
        return output_str


# ─────────────────────────────────────────────────────────────────────────────
# FILTER CHECKS
# ─────────────────────────────────────────────────────────────────────────────

# Keywords in starter code that indicate tree/list/graph problems
CUSTOM_CLASS_KEYWORDS = ['ListNode', 'TreeNode', 'Node']

def is_tree_list_problem(starter_code: str, input_strs: list[str]) -> bool:
    """Check if a problem uses custom data structures (trees, linked lists, graphs)."""
    # Check starter code for class references
    for keyword in CUSTOM_CLASS_KEYWORDS:
        if keyword in starter_code:
            return True
    # Check input strings for common tree/list parameter names
    for inp in input_strs:
        if any(name in inp for name in ['root =', 'head =', 'root1 =', 'root2 =',
                                         'head1 =', 'head2 =', 'node =', 'node1 =']):
            return True
    return False


def is_corrupted_output(output_str: str) -> bool:
    """Check if a test case output is an error message from the dataset generator."""
    if not output_str:
        return True
    output_str = output_str.strip()
    return (output_str.startswith('Error:') or
            output_str == 'Execution timed out' or
            output_str == '')


# ─────────────────────────────────────────────────────────────────────────────
# SNIPPET EXTRACTION
# ─────────────────────────────────────────────────────────────────────────────

# Map LeetCode's language names to our internal keys
LANG_MAP = {
    'Python3': 'python3',
    'Python': 'python',
    'JavaScript': 'javascript',
    'TypeScript': 'typescript',
    'Java': 'java',
    'C++': 'cpp',
    'C': 'c',
    'C#': 'csharp',
    'Go': 'golang',
    'Rust': 'rust',
    'Kotlin': 'kotlin',
    'Swift': 'swift',
    'Ruby': 'ruby',
    'PHP': 'php',
    'Dart': 'dart',
    'Scala': 'scala',
}

def extract_snippets(code_snippets: list[dict]) -> dict[str, str]:
    """Extract starter code snippets, mapped to normalized language keys."""
    templates = {}
    if not code_snippets:
        return templates
    for snippet in code_snippets:
        lang = snippet.get('lang', '')
        code = snippet.get('code', '')
        if lang in LANG_MAP and code:
            templates[LANG_MAP[lang]] = code
    return templates


# ─────────────────────────────────────────────────────────────────────────────
# MAIN COMPILATION
# ─────────────────────────────────────────────────────────────────────────────

def compile(limit: int = 0):
    # ── Download both datasets ─────────────────────────────────────────────
    api_data = download_leetcode_api()
    hf_data = download_hf_dataset()

    # ── Build lookup maps by question ID ───────────────────────────────────
    # leetcode-api: nested under data.question
    api_by_id = {}
    for entry in api_data:
        question = entry.get('data', {}).get('question', {})
        qid = question.get('questionId')
        if qid:
            api_by_id[str(qid)] = question

    # HF dataset: flat rows
    hf_by_id = {}
    for row in hf_data:
        qid = row.get('question_id')
        if qid is not None:
            hf_by_id[str(qid)] = row

    print(f"\nAPI problems indexed: {len(api_by_id)}")
    print(f"HF problems indexed:  {len(hf_by_id)}")

    # ── Inner join and compile ─────────────────────────────────────────────
    # Use HF dataset as the base (it has the test cases we need)
    compiled = []
    stats = {
        'matched': 0,
        'filtered_tree_list': 0,
        'filtered_corrupted': 0,
        'filtered_no_equals': 0,
        'filtered_no_tests': 0,
        'filtered_parse_fail': 0,
        'compiled_easy': 0,
        'compiled_medium': 0,
        'compiled_hard': 0,
    }

    hf_ids = sorted(hf_by_id.keys(), key=lambda x: int(x))

    for qid in hf_ids:
        if limit and len(compiled) >= limit:
            break

        hf_row = hf_by_id[qid]
        api_question = api_by_id.get(qid)
        
        if not api_question:
            continue  # No match in leetcode-api
        
        stats['matched'] += 1

        # ── Extract metadata from leetcode-api ─────────────────────────────
        title = api_question.get('title', f'Problem {qid}')
        difficulty = api_question.get('difficulty', 'Medium')
        description = api_question.get('content', '')
        code_snippets = api_question.get('codeSnippets', [])
        is_paid = api_question.get('isPaidOnly', False)
        
        # Skip paid/premium problems (no public description)
        if is_paid:
            continue

        # ── Extract snippets ───────────────────────────────────────────────
        starter_templates = extract_snippets(code_snippets)
        python_code = starter_templates.get('python3', starter_templates.get('python', ''))

        # ── Extract class_name and method_name from HF entry_point ─────────
        entry_point = hf_row.get('entry_point', '')
        # Format: "Solution().methodName"
        ep_match = re.match(r'(\w+)\(\)\.(\w+)', entry_point)
        if not ep_match:
            continue
        class_name = ep_match.group(1)
        method_name = ep_match.group(2)

        # ── Parse test cases from HF input_output ─────────────────────────
        io_raw = hf_row.get('input_output', '')
        try:
            io_list = json.loads(io_raw) if isinstance(io_raw, str) else io_raw
        except (json.JSONDecodeError, TypeError):
            stats['filtered_parse_fail'] += 1
            continue

        if not io_list or not isinstance(io_list, list):
            stats['filtered_no_tests'] += 1
            continue

        # ── Collect input strings for filter checks ────────────────────────
        input_strs = [tc.get('input', '') for tc in io_list]

        # ── FILTER: Tree/List problems ─────────────────────────────────────
        if is_tree_list_problem(python_code, input_strs):
            stats['filtered_tree_list'] += 1
            continue

        # ── FILTER: No-equals input format ─────────────────────────────────
        first_input = input_strs[0] if input_strs else ''
        if first_input and '=' not in first_input:
            stats['filtered_no_equals'] += 1
            continue

        # ── Parse and validate all test cases ──────────────────────────────
        parsed_inputs = []
        parsed_outputs = []
        had_corruption = False

        for tc in io_list:
            inp_str = tc.get('input', '')
            out_str = tc.get('output', '')

            # Skip corrupted test cases
            if is_corrupted_output(out_str):
                had_corruption = True
                continue

            # Parse input
            args = parse_input_string(inp_str)
            if args is None:
                continue

            # Parse output
            expected = parse_output_string(out_str)

            parsed_inputs.append(args)
            parsed_outputs.append(expected)

        # If ALL test cases were corrupted or unparseable, skip the problem
        if len(parsed_inputs) < 2:
            stats['filtered_corrupted'] += 1
            continue

        # ── Split into sample (first 2) and secret (rest) ─────────────────
        sample_inputs = parsed_inputs[:2]
        sample_outputs = parsed_outputs[:2]
        secret_inputs = parsed_inputs[2:]
        secret_outputs = parsed_outputs[2:]

        # ── Build the compiled problem object ──────────────────────────────
        problem = {
            'question_id': int(qid),
            'title': title,
            'difficulty': difficulty,
            'description': description,
            'class_name': class_name,
            'method_name': method_name,
            'starter_templates': starter_templates,
            'sample_inputs': sample_inputs,
            'sample_outputs': sample_outputs,
            'secret_inputs': secret_inputs,
            'secret_outputs': secret_outputs,
        }

        compiled.append(problem)

        # Track difficulty distribution
        diff_key = f"compiled_{difficulty.lower()}"
        if diff_key in stats:
            stats[diff_key] += 1

    # ── Write output ───────────────────────────────────────────────────────
    output_path = 'compiled_problems.json'
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(compiled, f, indent=2, ensure_ascii=False)

    # ── Print summary ──────────────────────────────────────────────────────
    print(f"\n{'='*60}")
    print(f"COMPILATION COMPLETE")
    print(f"{'='*60}")
    print(f"  Matched across both datasets:  {stats['matched']}")
    print(f"  Filtered (tree/list/graph):     {stats['filtered_tree_list']}")
    print(f"  Filtered (corrupted outputs):   {stats['filtered_corrupted']}")
    print(f"  Filtered (no-equals format):    {stats['filtered_no_equals']}")
    print(f"  Filtered (no valid tests):      {stats['filtered_no_tests']}")
    print(f"  Filtered (parse failures):      {stats['filtered_parse_fail']}")
    print(f"  -----------------------------------")
    print(f"  COMPILED problems:              {len(compiled)}")
    print(f"     Easy:    {stats['compiled_easy']}")
    print(f"     Medium:  {stats['compiled_medium']}")
    print(f"     Hard:    {stats['compiled_hard']}")
    print(f"\n  Output written to: {output_path}")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Compile LeetCode problem dataset')
    parser.add_argument('--limit', type=int, default=0,
                        help='Max problems to compile (0 = no limit)')
    args = parser.parse_args()
    compile(limit=args.limit)
