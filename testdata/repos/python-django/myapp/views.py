from django.http import JsonResponse


def index(request):
    return JsonResponse({'message': 'Hello from Django!'})


def health(request):
    return JsonResponse({'status': 'healthy'})
