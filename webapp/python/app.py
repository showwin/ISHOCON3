import datetime
import html
import os
import pathlib
import urllib

import MySQLdb.cursors

from flask import (
    Flask,
    abort,
    redirect,
    render_template,
    render_template_string,
    request,
    session
)

static_folder = pathlib.Path(__file__).resolve().parent / 'public'
app = Flask(__name__, static_folder=str(static_folder), static_url_path='')

app.secret_key = os.environ.get('ISHOCON3_SESSION_SECRET', 'showwin_happy')

_config = {
    'db_host': os.environ.get('ISHOCON3_DB_HOST', 'localhost'),
    'db_port': int(os.environ.get('ISHOCON3_DB_PORT', '3306')),
    'db_username': os.environ.get('ISHOCON3_DB_USER', 'ishocon'),
    'db_password': os.environ.get('ISHOCON3_DB_PASSWORD', 'ishocon'),
    'db_database': os.environ.get('ISHOCON3_DB_NAME', 'ishocon3'),
}


def config(key):
    if key in _config:
        return _config[key]
    else:
        raise "config value of %s undefined" % key


def db():
    if hasattr(request, 'db'):
        return request.db

    request.db = MySQLdb.connect(**{
        'host': config('db_host'),
        'port': config('db_port'),
        'user': config('db_username'),
        'passwd': config('db_password'),
        'db': config('db_database'),
        'charset': 'utf8mb4',
        'cursorclass': MySQLdb.cursors.DictCursor,
        'autocommit': True,
    })
    cur = request.db.cursor()
    cur.execute("SET SESSION sql_mode='TRADITIONAL,NO_AUTO_VALUE_ON_ZERO,ONLY_FULL_GROUP_BY'")
    cur.execute('SET NAMES utf8mb4')
    return request.db


@app.teardown_request
def close_db(exception=None):
    if hasattr(request, 'db'):
        request.db.close()


@app.route('/')
def get_index():
    return render_template_string('hello {{ what }}', what='world')


if __name__ == "__main__":
    app.run()
