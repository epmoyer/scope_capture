#!/usr/bin/env python

# Standard Library
from pathlib import Path
import subprocess
import os
import shutil

# library
import click


CONFIG = {
    # Application core
    'app_name': 'build',
    'output_app_name': 'scope_capture',
    'version': '0.1',
}

TARGETS = (
    # The first target is the "primary" target.  If `--all` is not specified then ONLY this
    # target will be built.
    # ('linux', 'amd64'),
    ('linux', 'arm64'),
    ('darwin', 'arm64'),
    # ('darwin', 'amd64'),
)

# We are scripts/release/build.py so .parent.parent.parent gets us to
# the project root directory.
PATH_PROJECT = Path(__file__).resolve().parent.parent.parent
PATH_CONFIG = PATH_PROJECT / 'pkg' / 'moduleconfig' / 'moduleconfig.go'
PATH_DIST = PATH_PROJECT / 'dist'
PATH_BUILDS = PATH_DIST / 'builds'

PATH_SCOPE_CAPTURE_SOURCE = PATH_PROJECT / 'cmd' / 'scope_capture'


@click.command()
@click.option('-a', '--all', 'build_all', is_flag=True, help='Build for all platforms')
def main(build_all):
    app_version = get_app_version()
    app_name = CONFIG['output_app_name']
    print(f'Building "{app_name}" version "{app_version}"...')

    if builds_exist(app_version):
        print(
            f'🔴 STOPPING. Builds already exist for version "{app_version}". '
            ' If you want to overwrite them then you must delete them first.'
        )
        return

    # Determine which targets to build
    if build_all:
        targets = TARGETS
    else:
        targets = [TARGETS[0]]

    # Build for each target
    for goos, goarch in targets:
        build_release(app_name, app_version, goos, goarch)


def builds_exist(version):
    return bool(list(PATH_BUILDS.glob(f'*_{version}_*')))


def build_release(app_name, version, goos, goarch):
    path_build_dir = PATH_BUILDS / f'{app_name}_{version}.{goarch}.{goos}'
    path_build_dir.mkdir(parents=True, exist_ok=True)

    # Because this app is "a single bin only", we can just copy the output binary into the
    # dist build directory.
    path_bin = path_build_dir
    path_bin.mkdir(parents=True, exist_ok=True)

    environment_mods = {'GOOS': goos, 'GOARCH': goarch}
    cmd = [
        "go",
        "build",
        "-o",
        f"{path_bin}/{app_name}.{goarch}.{goos}",
        ".",
    ]
    run(cmd, PATH_SCOPE_CAPTURE_SOURCE, environment_mods)


def ignore_patterns(path, names):
    ignore_names = ['.DS_Store', '__pycache__', '.gitkeep']
    ignore_list = []
    for name in names:
        if name in ignore_names:
            ignore_list.append(name)
            continue
        elif name.endswith('.pyc'):
            ignore_list.append(name)
            continue
        elif name.endswith('.log'):
            ignore_list.append(name)  # Exclude .log files
    return ignore_list


def get_app_version():
    version = None
    with open(PATH_CONFIG, 'r') as f:
        lines = f.readlines()
    for line in lines:
        if 'ModuleVersion' in line:
            version = line.split('"')[1]
            continue
    if version is None:
        raise RuntimeError(f'Version not found in "{PATH_CONFIG}".')
    return version


def run(cmd, cwd, environment_mods=None):
    message = f'RUNNING: "{" ".join(cmd)}"'
    env = None
    if environment_mods:
        env = os.environ.copy()
        for key, value in environment_mods.items():
            env[key] = value
        message += f'\n   WITH ENVIRONMENT MODS:{environment_mods}'
    print(message)
    subprocess.run(cmd, env=env, cwd=cwd)


if __name__ == '__main__':
    main()
