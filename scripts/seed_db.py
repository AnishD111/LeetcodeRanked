"""
seed_db.py — LeetCode Ranked Database Seeder

Reads compiled_problems.json and bulk-inserts all problems into the
PostgreSQL 'problems' table, replacing any existing seed data.

Usage:
    python seed_db.py [--db-url DATABASE_URL] [--file compiled_problems.json]
"""

import json
import sys
import os
import argparse

try:
    import psycopg2
    import psycopg2.extras
except ImportError:
    print("ERROR: psycopg2 is required. Install it with:")
    print("  pip install psycopg2-binary")
    sys.exit(1)


def seed(db_url: str, json_path: str):
    # ── Load compiled problems ─────────────────────────────────────────────
    print(f"Loading problems from {json_path}...")
    with open(json_path, 'r', encoding='utf-8') as f:
        problems = json.load(f)
    print(f"  -> {len(problems)} problems loaded")

    # ── Connect to PostgreSQL ──────────────────────────────────────────────
    print(f"Connecting to database...")
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    try:
        # ── Clear existing problems ────────────────────────────────────────
        # First check if there are foreign key references from matches
        cur.execute("SELECT COUNT(*) FROM matches WHERE problem_id IS NOT NULL")
        match_count = cur.fetchone()[0]
        if match_count > 0:
            print(f"  [Warning] {match_count} matches reference existing problems.")
            print(f"     Setting their problem_id to NULL before truncating...")
            cur.execute("UPDATE matches SET problem_id = NULL")

        cur.execute("DELETE FROM problems")
        print(f"  -> Cleared existing problems table")

        # ── Bulk insert ────────────────────────────────────────────────────
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
                if errors <= 5:
                    print(f"  [Error] Error inserting '{problem.get('title', '?')}': {e}")
                conn.rollback()
                # Re-start a fresh transaction for remaining inserts
                continue

        conn.commit()

        # ── Verify ─────────────────────────────────────────────────────────
        cur.execute("SELECT difficulty, COUNT(*) FROM problems GROUP BY difficulty ORDER BY difficulty")
        dist = cur.fetchall()

        print(f"\n{'='*50}")
        print(f"DATABASE SEED COMPLETE")
        print(f"{'='*50}")
        print(f"  Inserted: {inserted}")
        print(f"  Errors:   {errors}")
        print(f"\n  Difficulty Distribution:")
        for difficulty, count in dist:
            print(f"    {difficulty}: {count}")

    finally:
        cur.close()
        conn.close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Seed the problems database')
    parser.add_argument('--db-url', type=str,
                        default=os.environ.get('DATABASE_URL',
                                               'postgresql://postgres:postgres@localhost:5432/leetcode_ranked'),
                        help='PostgreSQL connection URL')
    parser.add_argument('--file', type=str, default='compiled_problems.json',
                        help='Path to compiled_problems.json')
    args = parser.parse_args()
    seed(args.db_url, args.file)
