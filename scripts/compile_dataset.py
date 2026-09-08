import ast
import json
import re
import sys
import urllib.request
import argparse
import warnings
from typing import Any, Optional

warnings.filterwarnings('ignore', category=SyntaxWarning)


def download_leetcode_api() -> list[dict]:
    url = "https://raw.githubusercontent.com/noworneverev/leetcode-api/main/data/leetcode_questions.json"
    print("Downloading leetcode-api questions from GitHub...")
    req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    with urllib.request.urlopen(req) as response:
        data = json.loads(response.read().decode('utf-8'))
    print(f"  Loaded {len(data)} questions.")
    return data


def download_hf_dataset() -> list[dict]:
    base_url = (
        "https://datasets-server.huggingface.co/rows?"
        "dataset=newfacade%2FLeetCodeDataset&config=default&split=train"
        "&offset={}&length=100"
    )
    all_rows = []
    offset = 0
    print("Downloading LeetCodeDataset from Hugging Face...")
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
            if len(all_rows) % 500 < 100:
                print(f"  {len(all_rows)} rows downloaded...")
        except Exception as e:
            print(f"  Stopped at offset {offset}: {e}")
            break
    print(f"  Total rows downloaded: {len(all_rows)}")
    return all_rows


def sanitize_for_json(obj: Any) -> Any:
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
    return obj


def parse_input_string(input_str: str) -> Optional[list]:
    if not input_str or '=' not in input_str:
        return None

    assignments = []
    pattern = re.compile(r'(?:^|, )(\w+)\s*=\s*')
    matches = list(pattern.finditer(input_str))

    if not matches:
        return None

    for i, match in enumerate(matches):
        value_start = match.end()
        if i + 1 < len(matches):
            next_match = matches[i + 1]
            value_end = next_match.start()
            value_str = input_str[value_start:value_end].rstrip(', ')
        else:
            value_str = input_str[value_start:].strip()

        try:
            value = ast.literal_eval(value_str)
            value = sanitize_for_json(value)
        except (ValueError, SyntaxError):
            value = value_str

        assignments.append(value)

    return assignments


def parse_output_string(output_str: str) -> Any:
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
        return output_str


CUSTOM_CLASS_KEYWORDS = ['ListNode', 'TreeNode', 'Node']


def is_tree_list_problem(starter_code: str, input_strs: list[str]) -> bool:
    for keyword in CUSTOM_CLASS_KEYWORDS:
        if keyword in starter_code:
            return True
    for inp in input_strs:
        if any(name in inp for name in ['root =', 'head =', 'root1 =', 'root2 =',
                                         'head1 =', 'head2 =', 'node =', 'node1 =']):
            return True
    return False


def is_corrupted_output(output_str: str) -> bool:
    if not output_str:
        return True
    output_str = output_str.strip()
    return (output_str.startswith('Error:') or
            output_str == 'Execution timed out' or
            output_str == '')


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
    templates = {}
    if not code_snippets:
        return templates
    for snippet in code_snippets:
        lang = snippet.get('lang', '')
        code = snippet.get('code', '')
        if lang in LANG_MAP and code:
            templates[LANG_MAP[lang]] = code
    return templates


def compile(limit: int = 0):
    api_data = download_leetcode_api()
    hf_data = download_hf_dataset()

    api_by_id = {}
    for entry in api_data:
        question = entry.get('data', {}).get('question', {})
        qid = question.get('questionId')
        if qid:
            api_by_id[str(qid)] = question

    hf_by_id = {}
    for row in hf_data:
        qid = row.get('question_id')
        if qid is not None:
            hf_by_id[str(qid)] = row

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
            continue

        stats['matched'] += 1

        title = api_question.get('title', f'Problem {qid}')
        difficulty = api_question.get('difficulty', 'Medium')
        description = api_question.get('content', '')
        code_snippets = api_question.get('codeSnippets', [])
        is_paid = api_question.get('isPaidOnly', False)

        if is_paid:
            continue

        starter_templates = extract_snippets(code_snippets)
        python_code = starter_templates.get('python3', starter_templates.get('python', ''))

        entry_point = hf_row.get('entry_point', '')
        ep_match = re.match(r'(\w+)\(\)\.(\w+)', entry_point)
        if not ep_match:
            continue
        class_name = ep_match.group(1)
        method_name = ep_match.group(2)

        io_raw = hf_row.get('input_output', '')
        try:
            io_list = json.loads(io_raw) if isinstance(io_raw, str) else io_raw
        except (json.JSONDecodeError, TypeError):
            stats['filtered_parse_fail'] += 1
            continue

        if not io_list or not isinstance(io_list, list):
            stats['filtered_no_tests'] += 1
            continue

        input_strs = [tc.get('input', '') for tc in io_list]

        if is_tree_list_problem(python_code, input_strs):
            stats['filtered_tree_list'] += 1
            continue

        first_input = input_strs[0] if input_strs else ''
        if first_input and '=' not in first_input:
            stats['filtered_no_equals'] += 1
            continue

        parsed_inputs = []
        parsed_outputs = []

        for tc in io_list:
            inp_str = tc.get('input', '')
            out_str = tc.get('output', '')

            if is_corrupted_output(out_str):
                continue

            args = parse_input_string(inp_str)
            if args is None:
                continue

            expected = parse_output_string(out_str)
            parsed_inputs.append(args)
            parsed_outputs.append(expected)

        if len(parsed_inputs) < 2:
            stats['filtered_corrupted'] += 1
            continue

        problem = {
            'question_id': int(qid),
            'title': title,
            'difficulty': difficulty,
            'description': description,
            'class_name': class_name,
            'method_name': method_name,
            'starter_templates': starter_templates,
            'sample_inputs': parsed_inputs[:2],
            'sample_outputs': parsed_outputs[:2],
            'secret_inputs': parsed_inputs[2:],
            'secret_outputs': parsed_outputs[2:],
        }

        compiled.append(problem)

        diff_key = f"compiled_{difficulty.lower()}"
        if diff_key in stats:
            stats[diff_key] += 1

    output_path = 'compiled_problems.json'
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(compiled, f, indent=2, ensure_ascii=False)

    print(f"Compilation complete. Total compiled: {len(compiled)}")
    print(f"  Easy: {stats['compiled_easy']}, Medium: {stats['compiled_medium']}, Hard: {stats['compiled_hard']}")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Compile LeetCode problem dataset')
    parser.add_argument('--limit', type=int, default=0, help='Max problems to compile (0 = no limit)')
    args = parser.parse_args()
    compile(limit=args.limit)
