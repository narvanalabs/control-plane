# Python Django

A Django application example.

## Install

```bash
pip install -r requirements.txt
```

## Run Development Server

```bash
python manage.py runserver
```

## Run Production Server

```bash
gunicorn myproject.wsgi:application --bind 0.0.0.0:8000
```

## Collect Static Files

```bash
python manage.py collectstatic --noinput
```
