from PIL import Image
import glob

files = sorted(glob.glob('slide_all-*.png'))
print('Total slides:', len(files))
print('%-8s %6s %7s %6s %7s %6s %8s %-20s' % ('Slide', 'Width', 'Height', 'Left', 'Right', 'Top', 'Bottom', 'Status'))
for f in files:
    img = Image.open(f)
    w, h = img.size
    # safe area excluding HUST theme header/footer (approx)
    safe_left, safe_right = 35, w - 35
    safe_top, safe_bottom = 65, h - 45
    # detect dark-ish content (text/images) inside safe area
    dark_threshold = 120
    minx, miny, maxx, maxy = safe_right, safe_bottom, safe_left, safe_top
    found = False
    for y in range(safe_top, safe_bottom, 2):
        for x in range(safe_left, safe_right, 2):
            p = img.getpixel((x,y))[:3]
            if max(p) < dark_threshold:  # dark pixel
                found = True
                minx = min(minx, x)
                miny = min(miny, y)
                maxx = max(maxx, x)
                maxy = max(maxy, y)
    if not found:
        print('%-8s %6d %7d no dark content' % (f, w, h))
        continue
    left = minx - safe_left
    top = miny - safe_top
    right = safe_right - maxx
    bottom = safe_bottom - maxy
    status = []
    threshold = 12
    if left < threshold: status.append('L')
    if right < threshold: status.append('R')
    if top < threshold: status.append('T')
    if bottom < threshold: status.append('B')
    print('%-8s %6d %7d %6d %7d %6d %8d %-20s' % (f, w, h, left, right, top, bottom, ','.join(status) if status else 'OK'))
