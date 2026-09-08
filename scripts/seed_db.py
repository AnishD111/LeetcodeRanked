import json
import sys
import os
import argparse

try:
    import psycopg2
    import psycopg2.extras
except ImportError:
    print("ERROR: psycopg2 is required. Install it with: pip install psycopg2-binary")
    sys.exit(1)


def seed(db_url: str, json_path: str):
    print(f"Loading problems from {json_path}...")
    with open(json_path, 'r', encoding='utf-8') as f:
        problems = json.load(f)
    print(f"Loaded {len(problems)} problems.")

    print("Connecting to database...")
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    try:
        cur.execute("SELECT COUNT(*) FROM matches WHERE problem_id IS NOT NULL")
        match_count = cur.fetchone()[0]
        if match_count > 0:
            cur.execute("UPDATE matches SET problem_id = NULL")

        cur.execute("DELETE FROM problems")

        insert_query = """
            INSERT INTO problems (
                title, difficulty, description, starter_templates,
                sample_inputs, sample_outputs,
                secret_inputs, secret_outputs,
                class_name, method_name
            ) VALUES (
                %(title)s, %(difficulty)s, %(description)s,
                %(starter_templates)s,
                %(sample_inputs)s, %(sample_outputs)s,
                %(secret_inputs)s, %(secret_outputs)s,
                %(class_name)s, %(method_name)s
            )
        """

        inserted = 0
        errors = 0

        for problem in problems:
            try:
                params = {
                    'title': problem['title'],
                    'difficulty': problem['difficulty'],
                    'description': problem['description'],
                    'starter_templates': json.dumps(problem['starter_templates']),
                    'sample_inputs': json.dumps(problem['sample_inputs']),
                    'sample_outputs': json.dumps(problem['sample_outputs']),
                    'secret_inputs': json.dumps(problem['secret_inputs']),
                    'secret_outputs': json.dumps(problem['secret_outputs']),
                    'class_name': problem['class_name'],
                    'method_name': problem['method_name'],
                }
                cur.execute(insert_query, params)
                inserted += 1
            except Exception as e:
                errors += 1
                conn.rollback()
                continue

        conn.commit()

        cur.execute("SELECT difficulty, COUNT(*) FROM problems GROUP BY difficulty ORDER BY difficulty")
        dist = cur.fetchall()

        print(f"Database seeded successfully. Inserted: {inserted}, Errors: {errors}")
        for difficulty, count in dist:
            print(f"  {difficulty}: {count}")

    finally:
        cur.close()
        conn.close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Seed the problems database')
    parser.add_argument('--db-url', type=str,
                        default=os.environ.get('DATABASE_URL', 'postgresql://postgres:postgres@localhost:5432/leetcode_ranked'),
                        help='PostgreSQL connection URL')
    parser.add_argument('--file', type=str, default='compiled_problems.json',
                        help='Path to compiled_problems.json')
    args = parser.parse_args()
    seed(args.db_url, args.file)
