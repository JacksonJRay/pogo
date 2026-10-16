SECRET_KEY = "pogo-example-not-a-secret"
DEBUG = True
ALLOWED_HOSTS = ["127.0.0.1"]
ROOT_URLCONF = "proj.urls"
MIDDLEWARE = []
INSTALLED_APPS = []
USE_TZ = True
DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": ":memory:",
    }
}
