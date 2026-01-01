"""URL configuration for myproject project."""
from django.urls import path, include

urlpatterns = [
    path('', include('myapp.urls')),
]
